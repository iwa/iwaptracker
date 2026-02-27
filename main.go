package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-co-op/gocron/v2"
)

func main() {
	// archipelago tracker which will periodically checks new items received for a slot, and send notifications on ntfy
	// 1. read config from env vars (period in minutes, archipelago room tracker id, slots ids)
	// 2. initial fetch of room & tracker data, to get games played & their datapackage hash (https://archipelago.gg/api/static_tracker/)
	// 3. fetch datapackage to build maps (id to items, id to locations)
	// 4. periodically fetch tracker data, and compare with previous data to detect new items received for each slot
	// 5. send notifications on ntfy for each new item received, with item name and location name

	// init all things
	db := InitDatabase("data/data.db")
	defer db.Close()

	state := NewState(db)

	config := NewConfig()
	parseEnvIntoConfig(config)

	// initial fetch + refresh
	initialFetch(config, state)
	log.Println("info: initial refresh of player data...")
	RefreshPlayerData(config, state)

	// create a scheduler
	log.Println("info: starting main loop with period of", config.PeriodMinutes, "minutes...")

	scheduler, err := gocron.NewScheduler()
	if err != nil {
		log.Panicln("error: could not create scheduler:", err)
	}
	defer scheduler.Shutdown()

	_, err = scheduler.NewJob(
		gocron.DurationJob(
			time.Duration(config.PeriodMinutes)*time.Minute,
		),
		gocron.NewTask(
			func() {
				RefreshPlayerData(config, state)
			},
		),
	)
	if err != nil {
		log.Panicln("error: could not create cron job:", err)
	}

	scheduler.Start()

	// block main thread until sigterm
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
	<-c
	log.Println("info: shutting down...")
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

	// room id parsing
	if roomID := os.Getenv("ROOM_ID"); roomID != "" {
		config.RoomID = roomID
		log.Println("info: room id is", roomID)
	} else {
		log.Panicln("error: ROOM_ID missing")
	}

	// watched slots ids parsing
	if slotIDs := os.Getenv("SLOT_IDS"); slotIDs != "" {
		splitted := strings.Split(slotIDs, ",")
		for _, v := range splitted {
			config.TrackedSlotIDs = append(config.TrackedSlotIDs, strings.TrimSpace(v))
		}

		log.Println("info: slot ids are", config.TrackedSlotIDs)
	} else {
		log.Panicln("error: SLOT_IDS missing")
	}

	// ntfy url parsing
	if ntfyURL := os.Getenv("NTFY_URL"); ntfyURL != "" {
		config.NtfyURL = ntfyURL
		log.Println("info: ntfy url is", ntfyURL)
	} else {
		log.Panicln("error: NTFY_URL missing")
	}
}

func initialFetch(config *Config, state *State) {

	// ROOM RELATED
	roomResp, err := http.Get("https://archipelago.gg/api/room_status/" + config.RoomID)
	if err != nil {
		log.Panicln("error: could not fetch room data:", err)
	}

	var roomStatusResponse RoomStatusResponse
	if err := json.NewDecoder(roomResp.Body).Decode(&roomStatusResponse); err != nil {
		log.Panicln("error: could not decode room response:", err)
	}

	// populate player id to player name map
	for i, playerData := range roomStatusResponse.Players {
		playerId := i + 1
		playerName := playerData[0]

		state.PlayerNamesMap[strconv.Itoa(playerId)] = playerName
	}

	config.TrackerID = roomStatusResponse.Tracker
	log.Println("info: tracker id is", config.TrackerID)

	// TRACKER RELATED

	trackerResp, err := http.Get("https://archipelago.gg/api/static_tracker/" + config.TrackerID)
	if err != nil {
		log.Panicln("error: could not fetch static tracker data:", err)
	}

	var apiStaticTrackerResponse StaticTrackerResponse
	if err := json.NewDecoder(trackerResp.Body).Decode(&apiStaticTrackerResponse); err != nil {
		log.Panicln("error: could not decode static tracker response:", err)
	}

	for _, playergame := range apiStaticTrackerResponse.PlayerGame {
		playerIDString := strconv.Itoa(playergame.Player)

		// populating player id to game name map
		state.PlayerGameMap[playerIDString] = playergame.Game
	}

	// fetch datapackages for needed games and populate state
	for gamename, datapackage := range apiStaticTrackerResponse.Datapackage {
		state.GamesMap[gamename] = *NewGame(gamename)

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
			state.GamesMap[gamename].IdItemsMap[strconv.Itoa(itemID)] = itemName
		}

		for locationName, locationID := range apiDatapackageResponse.LocationNameToID {
			state.GamesMap[gamename].IdLocationsMap[strconv.Itoa(locationID)] = locationName
		}

		log.Println("info: datapackage for game", gamename, "fetched and processed")

		time.Sleep(500 * time.Millisecond) // small delay to avoid hitting rate limits
	}
}

func RefreshPlayerData(config *Config, state *State) {
	log.Println("info: refreshing player data...")

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

		if !slices.Contains(config.TrackedSlotIDs, playerIDString) {
			continue // not tracked player, we ignore it
		}

		for _, items := range playerItemsReceived.Items {
			itemID := strconv.Itoa(items[0])
			locationID := strconv.Itoa(items[1])
			sentByPlayerID := strconv.Itoa(items[2])
			flagID := strconv.Itoa(items[3])

			// receiving player info
			gameName, ok := state.PlayerGameMap[playerIDString]
			if !ok {
				log.Println("warning: received item for player with unknown game, player id:", playerItemsReceived.Player, "skipping...")
				continue
			}

			game := state.GamesMap[gameName]

			itemName, ok := game.IdItemsMap[itemID]
			if !ok {
				log.Println("warning: received item with unknown id", itemID, "for game", game.Name)
			}

			// sender player info
			locationName := "?"
			senderGameName, ok := state.PlayerGameMap[sentByPlayerID]

			if ok {
				senderGame := state.GamesMap[senderGameName]

				locationName, ok = senderGame.IdLocationsMap[locationID]
				if !ok {
					log.Println("warning: received item with unknown location id", locationID, "for sender game", senderGame.Name)
					locationName = "?"
				}
			} else {
				log.Println("warning: unknown sender game, can't grab location name")
			}

			// check if the item was already received before for this slot
			alreadyReceived := false
			if slices.Contains(state.SlotReceivedItemsMap[playerIDString], itemID) {
				alreadyReceived = true
			}

			if alreadyReceived {
				continue // we already processed this item, we skip it
			}

			// new item received, we add it to the list of received items for this slot
			state.SlotReceivedItemsMap[playerIDString] = append(state.SlotReceivedItemsMap[playerIDString], itemID)
			SaveReceivedItem(state.DB, playerIDString, itemID)

			if sentByPlayerID == playerIDString {
				continue // do not trigger notification if it's an item unlocked by the player
			}

			if flagID != "1" && flagID != "2" && flagID != "3" {
				continue // only trigger notification for progression/useful items
			}

			// send notification on ntfy
			log.Println("info: new item received for player with slot id", playerItemsReceived.Player, ":", itemName, "at location", locationName)
			SendNotification(config, state, playerIDString, itemID, itemName, locationID, locationName, sentByPlayerID, flagID)
		}
	}

	log.Println("info: player data refreshed")
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
	case "4", "5", "6", "7":
		return "trap"
	default:
		return "unknown"
	}
}

func SendNotification(config *Config, state *State, playerID, itemID, itemName, locationID, locationName, sentByPlayerID, flagID string) {
	playerName, ok := state.PlayerNamesMap[playerID]
	if !ok {
		playerName = "id " + playerID
	}

	sentByPlayerName, ok := state.PlayerNamesMap[sentByPlayerID]
	if !ok {
		sentByPlayerName = "id " + sentByPlayerID
	}

	title := fmt.Sprintf("%s - Received %s (%s)", playerName, itemName, DetermineFlagRarity(flagID))
	message := fmt.Sprintf("item: %s (%s)\nlocation: %s (%s)\nsent by %s", itemName, itemID, locationName, locationID, sentByPlayerName)

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

	log.Println("info: response from Ntfy API:", string(responseBody))

	return nil
}
