package main

import "C"

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var roomList []*Room
var roomsMut sync.Mutex

var mysite site = site{siteURL: "http://172.16.230.1", siteOrigin: "http://172.16.230.1"}

func grabRoom(roomId int) *Room {

	var newRoom *Room = nil

	roomsMut.Lock()

	defer roomsMut.Unlock()

	//if room already exist return it

	for i := 0; i < len(roomList); i++ {

		if roomList[i].id == roomId {
			return roomList[i]
		}
	}

	//create new room

	sampleRate := 48000
	channels := 1
	latencyMS := 100

	newRoom = &Room{id: roomId, name: "my room", desc: "", RoomType: "", inputs: make([]*inputChannel, 0, 128), output: outputChannel{sampleRate: sampleRate, channels: channels, latencyMS: latencyMS, buffSize: (latencyMS * sampleRate * channels * 2) / 1000}}
	newRoom.ticker = time.NewTicker(time.Millisecond * time.Duration(latencyMS))

	go func(myroom *Room) {

		for t := range myroom.ticker.C {
			//Call the periodic function here.
			var buffers = myroom.mixOutputChannel(t)
			for _, mybuf := range buffers {
				myroom.writeClientChannel(mybuf)
			}
		}
	}(newRoom)

	roomList = append(roomList, newRoom)

	return newRoom
}

func tokenCheck(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("access-control-allow-credentials", "true")
	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.WriteHeader(200)

	//token := r.FormValue("token")
	token := r.Header.Get("CSRFtoken")

	err := mysite.checkCRSF(token)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("check CRSF Failed %v \n", err)))
		return
	}

	w.Write([]byte("check CRSF success"))

}

func handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	var err error
	var format string
	var roomID int

	roomID, err = strconv.Atoi(r.FormValue("roomID"))

	if err != nil {
		http.Error(w, "bad room id", http.StatusInternalServerError)
		return
	}

	format = r.FormValue("format")

	if len(format) <= 0 {
		format = "opus"
	}

	token := r.FormValue("token")

	/*
		err = mysite.newListener(roomID, token)
		if err != nil {
			http.Error(w, fmt.Sprintf("cannot create new Listener %v", err), http.StatusInternalServerError)
			return
		}
	*/

	room := grabRoom(roomID)

	if room == nil {
		http.Error(w, "room not found", http.StatusInternalServerError)
		return
	}

	if format == "wav" {
		w.Header().Set("content-type", "audio/wav")
	} else {
		w.Header().Set("content-type", "audio/ogg")
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(200)

	newClientId := room.addClient(w, token)
	client := room.getClient(newClientId)

	log.Printf("new client : %d in room [%d] %s ", client.id, room.id, room.name)

	defer room.removeClient(newClientId)

	e := getEncoder(format, w, room.output)
	err = e.writeHeader()

	if err != nil {
		http.Error(w, "error initializing audio encoder  ", http.StatusInternalServerError)
		return
	}

	for {
		newBuffer := <-client.channel
		if e.writeBuffer(newBuffer) != nil {
			break
		}
	}
}

// wsHandler implements the Handler Interface
type wsHandler struct{}

func (wsh wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	var err error
	var roomID int
	var myInputId int

	roomID, err = strconv.Atoi(r.FormValue("roomID"))

	if err != nil {
		http.Error(w, "not room id", http.StatusInternalServerError)
		return
	}

	token := r.FormValue("token")
	/*
		err = mysite.newInput(roomID, token)
		if err != nil {
			http.Error(w, fmt.Sprintf("mysite.newInput(%d,%s) API error %v", roomID, token, err), http.StatusInternalServerError)
			return
		}
	*/

	room := grabRoom(roomID)

	if room == nil {
		http.Error(w, "room not found", http.StatusInternalServerError)
		return
	}

	log.Printf("new audio input in %s \n", room.name)

	myInputId = room.addInput(48000, 1, token)
	myinput := room.getInput(myInputId)

	if myinput == nil {

		http.Error(w, "unable to add new input to room", http.StatusInternalServerError)
		return
	}

	defer room.removeInput(myInputId)

	// upgrader is needed to upgrade the HTTP Connection to a websocket Connection
	upgrader := &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	//Upgrading HTTP Connection to websocket connection
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("error upgrading %s", err)
		return
	}

	for {
		_, message, err := wsConn.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}

		myinput.totalRead += len(message)
		myinput.buffers = append(myinput.buffers, &inputChannelBuffer{buffer: message, nRead: 0})

	}

}

func main() {

	fmt.Println("goStream starting !")

	router := http.NewServeMux()
	router.Handle("/joinRoom", wsHandler{}) //handels websocket connections

	go func() {
		log.Fatal(http.ListenAndServe(":8080", router))
	}()

	http.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("./js"))))
	http.Handle("/html/", http.StripPrefix("/html/", http.FileServer(http.Dir("./html"))))

	http.HandleFunc("/tokenCheck", tokenCheck)
	http.HandleFunc("/joinRoom", handleJoinRoom)
	log.Fatal(http.ListenAndServe(":8081", nil))

}
