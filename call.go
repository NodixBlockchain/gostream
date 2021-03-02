package main

import (
	"log"
	"net/http"
	"strconv"
	"time"
)

type Message struct {
	messageType int
	fromToken   string
	callID      int
}

type messageClient struct {
	channel chan Message
	userID  int
}

func removeCall(id int) {

	callsMut.Lock()

	for idx, call := range callsList {

		if call.id == id {
			callsList[idx] = callsList[len(callsList)-1]
			callsList = callsList[:len(callsList)-1]
			break
		}
	}

	callsMut.Unlock()
}

func findCall(roomId int) *Room {

	callsMut.Lock()

	defer callsMut.Unlock()

	for i := 0; i < len(callsList); i++ {

		if callsList[i].id == roomId {
			return roomList[i]
		}
	}

	return nil
}

func messages(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Content-Type", "text/event-stream")

	if r.Method != "GET" {
		return
	}

	token := r.FormValue("CSRFtoken")

	var userid int

	if mysite.enable {

		var err error

		userid, err = mysite.checkCRSF(token)
		if err != nil {
			log.Printf("API mysite.checkCRSF(%s) \r\n", token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkCRSF API error", http.StatusForbidden)
			return
		}
	}

	newMessageClient := messageClient{channel: make(chan Message), userID: userid}
	messageClients = append(messageClients, &newMessageClient)

	w.Write([]byte("event: ping\ndata:ping\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	for {
		message := <-newMessageClient.channel

		var messageBody string

		if message.messageType == 1 {
			messageBody = "event: newCall\ndata: {\"from\": \"" + message.fromToken + "\"} \n\n"
		}
		if message.messageType == 2 {
			messageBody = "event: declineCall\ndata: {\"from\": \"" + message.fromToken + "\"} \n\n"
		}
		if message.messageType == 3 {
			messageBody = "event: acceptedCall\ndata: {\"from\": \"" + message.fromToken + "\", \"roomid\": " + strconv.Itoa(message.callID) + "} \n\n"
		}

		nWrote, err := w.Write([]byte(messageBody))
		if (err != nil) || (nWrote < len(messageBody)) {
			break
		}

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

	}

}

func newCall(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Access-Control-Allow-Headers", "CSRFToken")

	if r.Method != "GET" {
		w.WriteHeader(200)
		return
	}

	Destination, err := strconv.Atoi(r.FormValue("Destination"))

	if err != nil {
		http.Error(w, "bad Desination id", http.StatusInternalServerError)
		return
	}

	token := r.Header.Get("CSRFtoken")

	if mysite.enable {

		err := mysite.checkAppel(Destination, token)
		if err != nil {
			log.Printf("API  mysite.checkAppel(%d, %s) \r\n", Destination, token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkAppel API error", http.StatusForbidden)
			return
		}
	}

	for _, msglient := range messageClients {

		if msglient.userID == Destination {

			msglient.channel <- Message{messageType: 1, fromToken: token}
		}

	}
}

func rejectCall(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Access-Control-Allow-Headers", "CSRFToken")

	if r.Method != "GET" {
		w.WriteHeader(200)
		return
	}

	From, err := strconv.Atoi(r.FormValue("From"))

	if err != nil {
		http.Error(w, "bad source id", http.StatusInternalServerError)
		return
	}

	token := r.Header.Get("CSRFtoken")

	if mysite.enable {

		_, err := mysite.checkCRSF(token)
		if err != nil {
			log.Printf("API  mysite.checkCRSF(%s) \r\n", token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkAppel API error", http.StatusForbidden)
			return
		}
	}

	for _, msglient := range messageClients {

		if msglient.userID == From {

			msglient.channel <- Message{messageType: 2, fromToken: token}
		}

	}
}

func acceptCall(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Access-Control-Allow-Headers", "CSRFToken")

	if r.Method != "GET" {
		w.WriteHeader(200)
		return
	}

	From, err := strconv.Atoi(r.FormValue("From"))

	if err != nil {
		http.Error(w, "bad source id", http.StatusInternalServerError)
		return
	}

	token := r.Header.Get("CSRFtoken")

	var userid int

	if mysite.enable {

		userid, err = mysite.checkCRSF(token)
		if err != nil {
			log.Printf("API  mysite.checkCRSF(%s) \r\n", token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkAppel API error", http.StatusForbidden)
			return
		}

		err := mysite.checkAppel(From, token)
		if err != nil {
			log.Printf("API  mysite.checkAppel(%d, %s) \r\n", From, token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkAppel API error", http.StatusForbidden)
			return
		}
	}

	var newRoom *Room = nil

	callsMut.Lock()

	//if room already exist return it

	maxid := 0
	roomId := 0

	for i := 0; i < len(callsList); i++ {

		if (callsList[i].callFrom == From) && (callsList[i].callTo == userid) {
			roomId = callsList[i].id
			break
		}
		if (callsList[i].callFrom == userid) && (callsList[i].callTo == From) {
			roomId = callsList[i].id
			break
		}

		if callsList[i].id > maxid {
			maxid = callsList[i].id
		}
	}

	if roomId == 0 {

		roomId = maxid + 1

		//create new room

		sampleRate := 48000
		channels := 1
		latencyMS := 100

		newRoom = &Room{id: roomId, name: "call " + token + " from " + strconv.Itoa(From) + " ", callFrom: From, callTo: userid, RoomType: "", inputs: make([]*inputChannel, 0, 128), output: outputChannel{sampleRate: sampleRate, channels: channels, latencyMS: latencyMS, buffSize: (latencyMS * sampleRate * channels * 2) / 1000}}
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

		callsList = append(callsList, newRoom)

		callsMut.Unlock()
	}

	for _, msglient := range messageClients {

		if msglient.userID == From {

			msglient.channel <- Message{messageType: 3, fromToken: token, callID: roomId}
		}

	}

	w.Write([]byte("{\"roomid\": \"" + strconv.Itoa(roomId) + "\"}"))

}
