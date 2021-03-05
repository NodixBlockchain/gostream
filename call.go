package main

import (
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type Message struct {
	messageType int
	fromUID     int
}

type messageClient struct {
	channel chan Message
	userID  int
}

type Call struct {
	from int
	to   int

	inputs     [2]*inputChannel
	inputMut   sync.Mutex
	clients    [2]*roomClient
	clientsMut sync.Mutex

	output outputChannel
	ticker *time.Ticker
}

var callsList []*Call
var callsMut sync.Mutex

var messageClients []*messageClient
var msgClientsMut sync.Mutex

func removeCall(from int, to int) {

	callsMut.Lock()

	for idx, call := range callsList {

		if (call.from == from) && (call.to == to) {
			callsList[idx] = callsList[len(callsList)-1]
			callsList = callsList[:len(callsList)-1]
			break
		}
	}

	callsMut.Unlock()
}

func findCall(from int, to int) *Call {

	callsMut.Lock()

	defer callsMut.Unlock()

	for i := 0; i < len(callsList); i++ {

		if (callsList[i].from == from) && (callsList[i].to == to) {
			return callsList[i]
		}

		if (callsList[i].from == to) && (callsList[i].to == from) {
			return callsList[i]
		}
	}

	return nil
}
func getMsgClient(clientId int) *messageClient {

	msgClientsMut.Lock()

	defer msgClientsMut.Unlock()

	for i := 0; i < len(messageClients); i++ {

		if messageClients[i].userID == clientId {
			return messageClients[i]
		}
	}

	return nil
}

func removeMsgClient(id int) {

	msgClientsMut.Lock()

	for idx, client := range messageClients {

		if client.userID == id {
			messageClients[idx] = messageClients[len(messageClients)-1]
			messageClients = messageClients[:len(messageClients)-1]
			break
		}
	}

	msgClientsMut.Unlock()
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

	msgClientsMut.Lock()
	messageClients = append(messageClients, &newMessageClient)
	msgClientsMut.Unlock()

	log.Printf("new messages client (%s) \r\n", token)

	w.Write([]byte("event: ping\ndata:ping\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	for {
		message := <-newMessageClient.channel

		var messageBody string

		if message.messageType == 1 {
			messageBody = "event: newCall\ndata: {\"from\": " + strconv.Itoa(message.fromUID) + "} \n\n"
		}
		if message.messageType == 2 {
			messageBody = "event: declineCall\ndata: {\"from\": " + strconv.Itoa(message.fromUID) + "} \n\n"
		}
		if message.messageType == 3 {
			messageBody = "event: acceptedCall\ndata: {\"from\": " + strconv.Itoa(message.fromUID) + "} \n\n"
		}

		nWrote, err := w.Write([]byte(messageBody))
		if (err != nil) || (nWrote < len(messageBody)) {
			break
		}

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	log.Printf("lost messages client (%s) \r\n", token)

	removeMsgClient(userid)

}

func newCall(w http.ResponseWriter, r *http.Request) {

	var uid int

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

		uid, err = mysite.checkCRSF(token)
		if err != nil {
			log.Printf("API  mysite.checkCRSF(%s) \r\n", token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkCRSF API error", http.StatusForbidden)
			return
		}

		err = mysite.checkAppel(Destination, token)
		if err != nil {
			log.Printf("API  mysite.checkAppel(%d, %s) \r\n", Destination, token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkAppel API error", http.StatusForbidden)
			return
		}
	}

	msglient := getMsgClient(Destination)

	msglient.channel <- Message{messageType: 1, fromUID: uid}
}

func rejectCall(w http.ResponseWriter, r *http.Request) {

	var uid int

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

		uid, err = mysite.checkCRSF(token)
		if err != nil {
			log.Printf("API  mysite.checkCRSF(%s) \r\n", token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkCRSF API error", http.StatusForbidden)
			return
		}
	}

	msglient := getMsgClient(From)
	msglient.channel <- Message{messageType: 2, fromUID: uid}
}

func acceptCall(w http.ResponseWriter, r *http.Request) {

	var uid int

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

		uid, err = mysite.checkCRSF(token)
		if err != nil {
			log.Printf("API  mysite.checkCRSF(%s) \r\n", token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkAppel API error", http.StatusForbidden)
			return
		}

		err = mysite.checkAppel(From, token)
		if err != nil {
			log.Printf("API  mysite.checkAppel(%d, %s) \r\n", From, token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkAppel API error", http.StatusForbidden)
			return
		}
	}

	var newCall *Call = nil

	newCall = findCall(From, uid)

	//if room already exist return it

	if newCall == nil {

		//create new room
		callsMut.Lock()

		sampleRate := 48000
		channels := 1
		latencyMS := 100

		newCall = &Call{from: From, to: uid, output: outputChannel{sampleRate: sampleRate, channels: channels, latencyMS: latencyMS, buffSize: (latencyMS * sampleRate * channels * 2) / 1000}}
		newCall.ticker = time.NewTicker(time.Millisecond * time.Duration(latencyMS))

		callsList = append(callsList, newCall)
		callsMut.Unlock()

		go func(mycall *Call) {

			nSamples := mycall.output.buffSize / 2

			for _ = range mycall.ticker.C {

				buffer := make([]int16, nSamples, nSamples)

				for i := 0; i < 2; i++ {

					var myinput *inputChannel

					if i == 0 {
						myinput = mycall.inputs[0]
					} else {
						myinput = mycall.inputs[1]
					}

					if myinput == nil {
						continue
					}

					curBuffer := myinput.getBuffer()
					if curBuffer != nil {

						//iterate through room input samples to fill the buffer
						for nWriteChan := 0; nWriteChan < nSamples; nWriteChan++ {
							var inputSample int16

							curBuffer, inputSample = myinput.readSample(curBuffer)

							var newsample int32 = int32(buffer[nWriteChan]) + int32(inputSample)

							if newsample > 32767 {
								buffer[nWriteChan] = 32767
							} else if newsample < -32768 {
								buffer[nWriteChan] = -32768
							} else {
								buffer[nWriteChan] = int16(newsample)
							}

							if curBuffer == nil {
								break
							}
						}
					}

					if i == 0 && mycall.clients[1] != nil {
						if len(mycall.clients[1].channel) < 2 {
							mycall.clients[1].channel <- buffer
						}
					} else if mycall.clients[0] != nil {
						if len(mycall.clients[0].channel) < 2 {
							mycall.clients[0].channel <- buffer
						}
					}
				}
			}
		}(newCall)

	}

	getMsgClient(From).channel <- Message{messageType: 3, fromUID: uid}

}
