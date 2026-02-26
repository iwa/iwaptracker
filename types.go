package main

// Config struct
type Config struct {
	PeriodMinutes int
	TrackerID     string
	SlotIDs       []string

	NtfyURL string
}

func NewConfig() *Config {
	return &Config{}
}

// Game struct
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

// State struct, which holds all important information
type State struct {
	PlayerGameMap     map[string]string   // map player id to game name
	TrackedGames      map[string]Game     // map gamename to Game data
	SlotReceivedItems map[string][]string // map slot id to list of received items ids
}

func NewState() *State {
	return &State{
		TrackedGames:      make(map[string]Game),
		SlotReceivedItems: make(map[string][]string),
	}
}

/* --- DTO structs --- */

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

type TrackerResponse struct {
	PlayerItemsReceived []struct {
		Items  [][]int `json:"items"`
		Player int     `json:"player"`
		Team   int     `json:"team"`
	} `json:"player_items_received"`
}
