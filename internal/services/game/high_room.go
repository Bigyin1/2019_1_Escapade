package game

import (
	"escapade/internal/models"
	//re "escapade/internal/return_errors"
	//"math/rand"
)

// Game status
const (
	StatusPeopleFinding = iota
	StatusAborted       // in case of error
	StatusFlagPlacing
	StatusRunning
	StatusFinished
	StatusClosed
)

type RoomRequest struct {
	Connection *Connection `json:"connection"`
	Send       *RoomSend   `json:"send"`
	Get        *RoomGet    `json:"get"`
}

func (rr *RoomRequest) IsGet() bool {
	return rr.Get != nil
}

type RoomSend struct {
	Cell   *models.Cell `json:"cell"`
	Action *int         `json:"action"`
}

type RoomGet struct {
	players   bool `json:"players"`
	observers bool `json:"observers"`
	field     bool `json:"field"`
	history   bool `json:"history"`
}

type Room struct {
	Name   string `json:"name"`
	Status int    `json:"status"`

	players   *Connections `json:"players"`
	observers *Connections `json:"observers"`

	history []*PlayerAction `json:"history"`

	flags map[*Connection]*models.Cell `json:"-"`

	lobby *Lobby        `json:"-"`
	field *models.Field `json:"get"`

	chanLeave   chan *Connection  `json:"-"`
	chanRequest chan *RoomRequest `json:"-"`
}

func (room *Room) addAction(conn *Connection, action int) {
	pa := NewPlayerAction(conn.player, action)
	room.history = append(room.history, pa)
}

func (room *Room) SameAs(another *Room) bool {
	return room.field.SameAs(another.field)
}

// join handle user joining as player or observer
func (room *Room) Join(conn *Connection) bool {

	// if game not finish, lets check is that conn already in game
	if room.Status != StatusFinished {
		if room.alreadyPlaying(conn) {
			return true
		}
	}

	// reset old action and old points
	conn.player.Reset()

	// if room is searching new players
	if room.Status == StatusPeopleFinding {
		if room.EnterPlayer(conn) {
			return true
		}
	}

	// if you cant play, try observe
	if room.enterObserver(conn) {
		return true
	}

	return false
}

func (room *Room) Leave(conn *Connection) {

	// cant delete players, cause they always need
	// if game began
	if room.Status == StatusPeopleFinding {
		room.removeBeforeLaunch(conn)
	} else {
		room.removeAfterLaunch(conn)
	}
	room.addAction(conn, ActionDisconnect)
	room.sendTAIRPeople()
	return
}

func (room *Room) setFlag(conn *Connection, cell *models.Cell) bool {
	// if user try set flag after game launch
	if room.Status != StatusFlagPlacing {
		return false
	}

	if !room.field.IsInside(cell) {
		return false
	}
	room.flags[conn] = cell
	return true
}

// nanfle openCell
func (room *Room) openCell(conn *Connection, cell *models.Cell) bool {
	// if user try set open cell before game launch
	if room.Status != StatusRunning {
		return false
	}

	// if wrong cell
	if !room.field.IsInside(cell) {
		return false
	}

	// if user died
	if room.players.Get[conn] == true {
		return false
	}

	// set who try open cell(for history)
	cell.PlayerID = conn.GetPlayerID()
	room.field.OpenCell(cell)

	room.sendTAIRField()

	if room.field.IsCleared() {
		room.lobby.roomFinish(room)
	}
	return true
}

func (room *Room) cellHandle(conn *Connection, cell *models.Cell) (done bool) {
	if cell.Value == models.CellFlag {
		done = room.setFlag(conn, cell)
	} else {
		done = room.openCell(conn, cell)
	}
	return
}

func (room *Room) actionHandle(conn *Connection, action int) (done bool) {
	if action == ActionGiveUp {
		room.GiveUp(conn)
		return true
	}
	return false
}

func (room *Room) isInvalid(rr *RoomRequest) bool {
	return rr == nil || rr.Connection == nil || (rr.Get == nil && rr.Send == nil)
}

// handleRequest
func (room *Room) handleRequest(rr *RoomRequest) {
	if room.isInvalid(rr) {
		return
	}

	if rr.IsGet() {
		room.requestGet(rr)
	} else {
		done := false
		if rr.Send.Cell != nil {
			done = room.cellHandle(rr.Connection, rr.Send.Cell)
		} else if rr.Send.Action != nil {
			done = room.actionHandle(rr.Connection, *rr.Send.Action)
		}
		if !done {
			sendError(rr.Connection, "room request", "Cant execute request ")
		}
	}
}

func (room *Room) startFlagPlacing() {
	room.Status = StatusFlagPlacing
	room.lobby.roomStart(room)
	room.fillField()
}

func (room *Room) startGame() {
	room.Status = StatusRunning
	room.fillField()
}

// Run the room in goroutine
func (room *Room) run() {
	//timer := time.NewTimer()
	for {
		select {

		case connection := <-room.chanLeave:
			room.Leave(connection)
		case request := <-room.chanRequest:
			room.handleRequest(request)
		}
	}
}