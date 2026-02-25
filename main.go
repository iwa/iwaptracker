package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
)

type Config struct {
	PeriodMinutes int
	TrackerID     string
	SlotIDs       []string

	NtfyURL string
}

func NewConfig() *Config {
	return &Config{}
}

type Game struct {
	Name           string
	IdItemsMap     map[string]string
	IdLocationsMap map[string]string
}

func NewGame(name string) *Game {
	return &Game{
		Name:           name,
		IdItemsMap:     make(map[string]string),
		IdLocationsMap: make(map[string]string),
	}
}

type State struct {
	PlayerGameMap     map[string]string   // map of player id to game name
	TrackedGames      map[string]Game     // map gamename to Game data
	SlotReceivedItems map[string][]string // map of slot id to list of received items ids
}

func NewState() *State {
	return &State{
		TrackedGames:      make(map[string]Game),
		SlotReceivedItems: make(map[string][]string),
	}
}

func main() {
	// archipelago tracker which will periodically checks new items received for a slot, and send notifications on ntfy
	// 1. read config from env vars (period in minutes, archipelago room tracker id, slots ids)
	// 2. initial fetch of room & tracker data, to get games played & their datapackage hash (https://archipelago.gg/api/static_tracker/)
	// 3. fetch datapackage to build maps (id to items, id to locations)
	// 4. periodically fetch tracker data, and compare with previous data to detect new items received for each slot
	// 5. send notifications on ntfy for each new item received, with item name and location name

	state := NewState()

	config := NewConfig()
	parseEnvIntoConfig(config)

	initialFetch(config, state)

	log.Println("info: starting main loop with period of", config.PeriodMinutes, "minutes...")

}

func parseEnvIntoConfig(config *Config) {
	// period parsing
	if period := os.Getenv("PERIOD_MINUTES"); period != "" {
		var err error
		config.PeriodMinutes, err = strconv.Atoi(period)

		if err != nil {
			log.Println("warning: could not parse the period value, using default 60 minutes...")
			config.PeriodMinutes = 60
		}
	} else {
		log.Println("warning: PERIOD_MINUTES env var not set, using default 60 minutes...")
		config.PeriodMinutes = 60
	}

	// tracker id parsing
	if trackerID := os.Getenv("TRACKER_ID"); trackerID != "" {
		config.TrackerID = trackerID
		log.Println("info: tracker id is", trackerID)
	} else {
		log.Panicln("error: TRACKER_ID missing")
	}

	// slots ids parsing
	if slotIDs := os.Getenv("SLOT_IDS"); slotIDs != "" {
		splitted := strings.Split(slotIDs, ",")
		for _, v := range splitted {
			config.SlotIDs = append(config.SlotIDs, strings.TrimSpace(v))
		}

		log.Println("info: slot ids are", config.SlotIDs)
	} else {
		log.Panicln("error: SLOT_IDS missing")
	}
}

type StaticTrackerResponse struct {
	Datapackage map[string]struct {
		Checksum string `json:"checksum"`
		Version  int    `json:"version"`
	} `json:"datapackage"`
	PlayerGame []struct {
		Game   string `json:"game"`
		Player int    `json:"player"`
		Team   int    `json:"team"`
	} `json:"player_game"`
}

type DatapackageResponse struct {
	Checksum           string              `json:"checksum"`
	ItemNameGroups     map[string][]string `json:"item_name_groups"`
	ItemNameToID       map[string]int      `json:"item_name_to_id"`
	LocationNameGroups map[string][]string `json:"location_name_groups"`
	LocationNameToID   map[string]int      `json:"location_name_to_id"`
}

func initialFetch(config *Config, state *State) {

	resp, err := http.Get("https://archipelago.gg/api/static_tracker/" + config.TrackerID)
	if err != nil {
		log.Panicln("error: could not fetch static tracker data:", err)
	}

	var apiStaticTrackerResponse StaticTrackerResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiStaticTrackerResponse); err != nil {
		log.Panicln("error: could not decode static tracker response:", err)
	}

	// store games played by tracked players only
	gameSet := make(map[string]bool)

	for _, playergame := range apiStaticTrackerResponse.PlayerGame {
		playerIDString := strconv.Itoa(playergame.Player)

		if !slices.Contains(config.SlotIDs, playerIDString) {
			continue // not tracked player, we ignore it
		}

		// we store the game played for this player, to fetch the datapackage later
		state.PlayerGameMap[playerIDString] = playergame.Game

		// add game to game set to fetch datapackage after
		if _, ok := gameSet[playergame.Game]; !ok {
			gameSet[playergame.Game] = true
		}
	}

	// fetch datapackages for needed games and populate state
	for gamename, datapackage := range apiStaticTrackerResponse.Datapackage {
		if _, ok := gameSet[gamename]; ok {

			state.TrackedGames[gamename] = *NewGame(gamename)

			log.Println("info: fetching datapackage for game", gamename)

			datapackageResp, err := http.Get("https://archipelago.gg/api/datapackage/" + datapackage.Checksum)
			if err != nil {
				log.Panicln("error: could not fetch datapackage data for game", gamename, ":", err)
			}

			var apiDatapackageResponse DatapackageResponse
			if err := json.NewDecoder(datapackageResp.Body).Decode(&apiDatapackageResponse); err != nil {
				log.Panicln("error: could not decode datapackage response for game", gamename, ":", err)
			}

			for itemName, itemID := range apiDatapackageResponse.ItemNameToID {
				state.TrackedGames[gamename].IdItemsMap[strconv.Itoa(itemID)] = itemName
			}

			for locationName, locationID := range apiDatapackageResponse.LocationNameToID {
				state.TrackedGames[gamename].IdLocationsMap[strconv.Itoa(locationID)] = locationName
			}

			log.Println("info: datapackage for game", gamename, "fetched and processed")
		}
	}
}

type TrackerResponse struct {
	PlayerItemsReceived []struct {
		Items  [][]int `json:"items"`
		Player int     `json:"player"`
		Team   int     `json:"team"`
	} `json:"player_items_received"`
}

func RefreshPlayerData(config *Config, state *State) {
	resp, err := http.Get("https://archipelago.gg/api/tracker/" + config.TrackerID)
	if err != nil {
		log.Println("error: could not fetch tracker data, skipping...", err)
		return
	}

	var apiTrackerResponse TrackerResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiTrackerResponse); err != nil {
		log.Println("error: could not decode tracker response, skipping...", err)
		return
	}

	for _, playerItemsReceived := range apiTrackerResponse.PlayerItemsReceived {
		playerIDString := strconv.Itoa(playerItemsReceived.Player)

		if !slices.Contains(config.SlotIDs, playerIDString) {
			continue // not tracked player, we ignore it
		}

		for _, items := range playerItemsReceived.Items {
			itemID := strconv.Itoa(items[0])
			locationID := strconv.Itoa(items[1])
			sentByPlayerID := strconv.Itoa(items[2])
			flagID := strconv.Itoa(items[3])

			gameName, ok := state.PlayerGameMap[playerIDString]
			if !ok {
				log.Println("warning: received item for player with unknown game, player id:", playerItemsReceived.Player, "skipping...")
				continue
			}

			game := state.TrackedGames[gameName]

			itemName, ok := game.IdItemsMap[itemID]
			if !ok {
				log.Println("warning: received item with unknown id", itemID, "for game", game.Name)
			}

			locationName, ok := game.IdLocationsMap[locationID]
			if !ok {
				log.Println("warning: received item with unknown location id", locationID, "for game", game.Name)
			}

			// check if the item was already received before for this slot
			alreadyReceived := false
			if slices.Contains(state.SlotReceivedItems[playerIDString], itemID) {
				alreadyReceived = true
			}

			if alreadyReceived {
				continue // we already processed this item, we skip it
			}

			// new item received, we add it to the list of received items for this slot
			state.SlotReceivedItems[playerIDString] = append(state.SlotReceivedItems[playerIDString], itemID)

			// send notification on ntfy
			log.Println("info: new item received for player with slot id", playerItemsReceived.Player, ":", itemName, "at location", locationName)
			SendNotification(config, playerIDString, itemID, itemName, locationID, locationName, sentByPlayerID, flagID)
		}
	}
}

func DetermineFlagRarity(flagID string) string {
	switch flagID {
	case "0":
		return "normal"
	case "1":
		return "progression"
	case "2":
		return "useful"
	case "3":
		return "progression + useful"
	case "4":
		return "trap"
	default:
		return "unknown"
	}
}

func SendNotification(config *Config, playerID, itemID, itemName, locationID, locationName, sentByPlayerID, flagID string) {
	title := fmt.Sprintf("%s - Received %s (%s)", playerID, itemName, DetermineFlagRarity(flagID))
	message := fmt.Sprintf("item: %s (%s)\nlocation: %s (%s)\nby player %s", itemName, itemID, locationName, locationID, playerID)

	err := SendNtfy(config, title, message)
	if err != nil {
		log.Println("error: could not send notification for player id", playerID, "item id", itemID, ":", err)
	}
}

func SendNtfy(config *Config, title, message string) error {
	req, err := http.NewRequest("POST", config.NtfyURL, strings.NewReader(message))
	if err != nil {
		return err
	}

	req.Header.Set("Title", title)
	req.Header.Set("Priority", "default")
	req.Header.Set("Tags", "sparkles")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	fmt.Println("[INFO] Response from Ntfy API:", string(responseBody))

	return nil
}
