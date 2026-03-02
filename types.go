package main

import "database/sql"

// Config struct
type Config struct {
	PeriodMinutes  int
	RoomID         string
	TrackerID      string
	TrackedSlotIDs []string

	NtfyURL           string
	DiscordWebhookURL string

	SignalMessageURL string
	SignalNumber     string
	SignalRecipient  string
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
	DB                   *sql.DB
	PlayerNamesMap       map[string]string   // map player id to player name
	PlayerGameMap        map[string]string   // map player id to game name
	GamesMap             map[string]Game     // map gamename to Game data
	SlotReceivedItemsMap map[string][]string // map slot id to list of received items ids
}

func NewState(db *sql.DB) *State {
	return &State{
		DB:                   db,
		PlayerNamesMap:       make(map[string]string),
		PlayerGameMap:        make(map[string]string),
		GamesMap:             make(map[string]Game),
		SlotReceivedItemsMap: LoadReceivedItems(db),
	}
}

func (s *State) GetPlayerName(playerID string) string {
	playerName, ok := s.PlayerNamesMap[playerID]
	if !ok {
		return ""
	}
	return playerName
}

func (s *State) GetGameNameByPlayerId(playerID string) string {
	gameName, ok := s.PlayerGameMap[playerID]
	if !ok {
		return ""
	}
	return gameName
}

func (s *State) GetGameByName(gameName string) (Game, bool) {
	game, ok := s.GamesMap[gameName]
	return game, ok
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

type RoomStatusResponse struct {
	LastActivity string     `json:"last_activity"`
	LastPort     int        `json:"last_port"`
	Players      [][]string `json:"players"`
	Timeout      int        `json:"timeout"`
	Tracker      string     `json:"tracker"`
}
