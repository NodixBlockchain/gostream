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

var mysite site = site{siteURL: "http://172.16.230.1", siteOrigin: "http://172.16.230.1", enable: true}

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
	w.Header().Set("Access-Control-Allow-Headers", "CSRFToken")

	w.WriteHeader(200)

	if r.Method != "GET" {
		log.Println("token check headers sent ")
		return
	}

	log.Println("token check request ")

	//token := r.FormValue("token")
	token := r.Header.Get("CSRFtoken")

	err := mysite.checkCRSF(token)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("check CRSF Failed %s %v \r\n", token, err)))
		log.Printf("token %s check error %v ", token, err)
		return
	}

	w.Write([]byte("check CRSF success\r\n"))

}

func handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	var err error
	var format string
	var roomID int

	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Access-Control-Allow-Headers", "CSRFToken")

	if r.Method != "GET" {
		w.WriteHeader(200)
		return
	}

	roomID, err = strconv.Atoi(r.FormValue("roomID"))

	if err != nil {
		http.Error(w, "bad room id", http.StatusInternalServerError)
		return
	}

	format = r.FormValue("format")

	if len(format) <= 0 {
		format = "opus"
	}

	token := r.Header.Get("CSRFtoken")
	if token == "" {
		token = r.FormValue("token")
	}

	if mysite.enable {
		err = mysite.newListener(roomID, token, 1)
		if err != nil {
			log.Printf("API mysite.newListener(%d,%s,1) \r\n", roomID, token)
			log.Println("error ", err)

			http.Error(w, "mysite.newListener API error", http.StatusForbidden)
			return
		}
	}

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

	w.WriteHeader(200)

	newClientId := room.addClient(w, token)
	client := room.getClient(newClientId)

	log.Printf("new client : %d in room [%d] %s using token '%s'", client.id, room.id, room.name, token)

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

	if mysite.enable {
		err = mysite.newListener(roomID, token, 0)
		if err != nil {
			log.Printf("API mysite.newListener(%d,%s,0) \r\n", roomID, token)
			log.Println("error ", err)

			http.Error(w, "mysite.newListener API error", http.StatusForbidden)
			return
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

	if mysite.enable {

		err = mysite.newInput(roomID, token, 1)
		if err != nil {
			log.Printf("API mysite.newInput(%d,%s,1) \r\n", roomID, token)
			log.Println("error : ", err)

			http.Error(w, "mysite.newInput API error", http.StatusForbidden)
			return
		}
	}

	room := grabRoom(roomID)

	if room == nil {
		http.Error(w, "room not found", http.StatusInternalServerError)
		return
	}

	log.Printf("new audio input in %s using token '%s'\n", room.name, token)

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

	if mysite.enable {

		err = mysite.newInput(roomID, token, 0)
		if err != nil {
			log.Printf("API mysite.newInput(%d,%s,0) \r\n", roomID, token)
			log.Println("error : ", err)

			http.Error(w, "mysite.newInput API error", http.StatusForbidden)
			return
		}
	}

}

/*
func newCRSF(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("content-type", "application/json")
	w.Write([]byte("{\"token\" : \"123456token\"}"))

}
func crossLogin(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("content-type", "application/json")
	w.Write([]byte("1"))

}

func envoieAudioGroup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")
	w.Write([]byte("1"))
}

func ecouteAudioGroup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")

	//w.Write([]byte(r.URL.Path))
	w.Write([]byte("1"))
}
*/
func main() {

	fmt.Println("goStream starting !")

	/*
		routerSite := mux.NewRouter()

		routerSite.HandleFunc("/Membres/crossLogin/{token}", crossLogin)
		routerSite.HandleFunc("/Membres/newCRSF", newCRSF)
		routerSite.HandleFunc("/Groupes/envoieAudioGroup/{roomid:[0-9]+}/{token:[a-zA-Z0-9]+}/{on:[0-9]+}", envoieAudioGroup)
		routerSite.HandleFunc("/Groupes/ecouteAudioGroup/{roomid:[0-9]+}/{token:[a-zA-Z0-9]+}/{on:[0-9]+}", ecouteAudioGroup)

		routerSite.Handle("/js/{file}", http.StripPrefix("/js/", http.FileServer(http.Dir("./js"))))
		routerSite.Handle("/html/", http.StripPrefix("/html/", http.FileServer(http.Dir("./html"))))


		go func() {
			log.Fatal(http.ListenAndServe(":80", routerSite))
		}()
	*/

	router := http.NewServeMux()
	router.Handle("/upRoom", wsHandler{}) //handels websocket connections
	router.HandleFunc("/joinRoom", handleJoinRoom)

	router.HandleFunc("/tokenCheck", tokenCheck)

	router.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("./js"))))
	router.Handle("/html/", http.StripPrefix("/html/", http.FileServer(http.Dir("./html"))))

	log.Fatal(http.ListenAndServe(":8080", router))

}
