package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type Message struct {
	messageType int

	challenge string
	answer    string

	fromUID    int
	fromPubKey *ecdsa.PublicKey
}

type messageClient struct {
	channel chan Message
	w       http.ResponseWriter

	userID int
	pubKey *ecdsa.PublicKey
}

type Call struct {
	from int
	to   int

	toPKEY   *ecdsa.PublicKey
	fromPKEY *ecdsa.PublicKey

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

func findCallPKey(from *ecdsa.PublicKey, to *ecdsa.PublicKey) *Call {

	callsMut.Lock()

	defer callsMut.Unlock()

	for i := 0; i < len(callsList); i++ {

		if callsList[i].fromPKEY.Equal(from) && callsList[i].toPKEY.Equal(to) {
			return callsList[i]
		}

		if callsList[i].fromPKEY.Equal(to) && callsList[i].toPKEY.Equal(from) {
			return callsList[i]
		}
	}

	return nil
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
func sendMsgClient(clientId int, message Message) error {

	msgClientsMut.Lock()

	defer msgClientsMut.Unlock()

	for i := 0; i < len(messageClients); i++ {

		if messageClients[i].userID == clientId {
			messageClients[i].channel <- message
			return nil
		}
	}

	return fmt.Errorf("client not found")
}

func sendMsgClientPkey(clientKey *ecdsa.PublicKey, message Message) error {

	msgClientsMut.Lock()

	defer msgClientsMut.Unlock()

	for i := 0; i < len(messageClients); i++ {

		if messageClients[i].pubKey.Equal(clientKey) {
			messageClients[i].channel <- message
			return nil
		}
	}

	return fmt.Errorf("client not found")
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

func removeMsgClientPKey(pkey *ecdsa.PublicKey) {

	msgClientsMut.Lock()

	for idx, client := range messageClients {

		if client.pubKey.Equal(pkey) {
			messageClients[idx] = messageClients[len(messageClients)-1]
			messageClients = messageClients[:len(messageClients)-1]
			break
		}
	}

	msgClientsMut.Unlock()
}

func pubKeyFromText(text string, format string) (*ecdsa.PublicKey, error) {
	var err error
	var k []byte

	if format == "hex" {
		k, err = hex.DecodeString(text)
		if err != nil {
			return nil, err
		}
	}
	X, Y := elliptic.UnmarshalCompressed(privateKey.Curve, k)
	if X == nil || Y == nil {
		return nil, fmt.Errorf("bad point")
	}
	return &ecdsa.PublicKey{Curve: privateKey.Curve, X: X, Y: Y}, nil
}

func messages(w http.ResponseWriter, r *http.Request) {

	var newMessageClient *messageClient

	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Content-Type", "text/event-stream")

	if r.Method != "GET" {
		return
	}

	if mysite.enable {

		var err error

		token := r.FormValue("CSRFtoken")

		userid, err := mysite.checkCRSF(token)
		if err != nil {
			log.Printf("API mysite.checkCRSF(%s) \r\n", token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkCRSF API error", http.StatusForbidden)
			return
		}
		newMessageClient = &messageClient{w: w, channel: make(chan Message), userID: userid, pubKey: nil}
		log.Printf("new messages client (%s) \r\n", token)
	} else {
		srcpub, err := pubKeyFromText(r.FormValue("PKey"), "hex")
		if err != nil {
			http.Error(w, "bad PKey", http.StatusForbidden)
			return
		}
		newMessageClient = &messageClient{w: w, channel: make(chan Message), userID: 0, pubKey: srcpub}
		log.Printf("new messages client (%x) \r\n", srcpub)
	}

	msgClientsMut.Lock()
	messageClients = append(messageClients, newMessageClient)
	msgClientsMut.Unlock()

	w.Write([]byte("event: ping\ndata:ping\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	for {
		message := <-newMessageClient.channel

		var messageBody string

		if message.messageType == 1 {
			messageBody = "event: newCall\ndata: {"
		}
		if message.messageType == 2 {
			messageBody = "event: declineCall\ndata: {"
		}
		if message.messageType == 3 {
			messageBody = "event: acceptedCall\ndata: {"
		}
		if message.messageType == 4 {
			messageBody = "event: answer\ndata: {"
			messageBody += "\"challenge\": \"" + message.challenge + "\","
			messageBody += "\"answer\": \"" + message.answer + "\","
		}
		if message.messageType == 5 {
			messageBody = "event: answer2\ndata: {"
			messageBody += "\"challenge\": \"" + message.challenge + "\","
			messageBody += "\"answer\": \"" + message.answer + "\","
		}

		if message.fromUID != 0 {
			messageBody += "\"from\": " + strconv.Itoa(message.fromUID)
		} else {
			if message.messageType == 1 {
				messageBody += "\"challenge\": \"" + message.challenge + "\","
			}
			if message.messageType == 2 || message.messageType == 3 {
				messageBody += "\"answer\": \"" + message.answer + "\","
			}
			messageBody += "\"from\": \"" + hex.EncodeToString(elliptic.MarshalCompressed(message.fromPubKey.Curve, message.fromPubKey.X, message.fromPubKey.Y)) + "\""

		}

		messageBody += "} \n\n"

		nWrote, err := w.Write([]byte(messageBody))
		if (err != nil) || (nWrote < len(messageBody)) {
			break
		}

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	if newMessageClient.userID > 0 {
		log.Printf("lost messages client (%d) \r\n", newMessageClient.userID)
		removeMsgClient(newMessageClient.userID)
	} else {
		log.Printf("lost messages client (%x) \r\n", newMessageClient.pubKey)
		removeMsgClientPKey(newMessageClient.pubKey)
	}

}

func newCall(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Access-Control-Allow-Headers", "PKey,CSRFToken")

	if r.Method != "POST" {
		w.WriteHeader(200)
		return
	}

	Destination := r.FormValue("Destination")

	if mysite.enable {

		token := r.Header.Get("CSRFtoken")

		uid, err := mysite.checkCRSF(token)
		if err != nil {
			log.Printf("API  mysite.checkCRSF(%s) \r\n", token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkCRSF API error", http.StatusForbidden)
			return
		}

		destID, err := strconv.Atoi(Destination)

		if err != nil {
			http.Error(w, "bad destination id", http.StatusForbidden)
			return
		}

		err = mysite.checkAppel(destID, token)
		if err != nil {
			log.Printf("API  mysite.checkAppel(%d, %s) \r\n", destID, token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkAppel API error", http.StatusForbidden)
			return
		}
		sendMsgClient(destID, Message{messageType: 1, fromUID: uid})
	} else {

		dstpub, err := pubKeyFromText(Destination, "hex")
		if err != nil {
			http.Error(w, "bad Destination", http.StatusForbidden)
			return
		}

		srcpub, err := pubKeyFromText(r.Header.Get("PKey"), "hex")
		if err != nil {
			http.Error(w, "bad PKey", http.StatusForbidden)
			return
		}

		Challenge := r.FormValue("challenge")

		sendMsgClientPkey(dstpub, Message{messageType: 1, challenge: Challenge, fromUID: 0, fromPubKey: srcpub})
	}
}

func answer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Access-Control-Allow-Headers", "PKey")

	if r.Method != "POST" {
		w.WriteHeader(200)
		return
	}

	dstpub, err := pubKeyFromText(r.FormValue("Destination"), "hex")
	if err != nil {
		http.Error(w, "bad Destination", http.StatusForbidden)
		return
	}

	srcpub, err := pubKeyFromText(r.Header.Get("PKey"), "hex")
	if err != nil {
		http.Error(w, "bad PKey", http.StatusForbidden)
		return
	}

	Signature := r.FormValue("signature")
	Challenge := r.FormValue("challenge")

	sendMsgClientPkey(dstpub, Message{messageType: 4, answer: Signature, challenge: Challenge, fromUID: 0, fromPubKey: srcpub})

}

func answer2(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Access-Control-Allow-Headers", "PKey")

	if r.Method != "POST" {
		w.WriteHeader(200)
		return
	}

	dstpub, err := pubKeyFromText(r.FormValue("Destination"), "hex")
	if err != nil {
		http.Error(w, "bad Destination", http.StatusForbidden)
		return
	}

	srcpub, err := pubKeyFromText(r.Header.Get("PKey"), "hex")
	if err != nil {
		http.Error(w, "bad PKey", http.StatusForbidden)
		return
	}

	Signature := r.FormValue("signature")
	Challenge := r.FormValue("challenge")

	sendMsgClientPkey(dstpub, Message{messageType: 5, answer: Signature, challenge: Challenge, fromUID: 0, fromPubKey: srcpub})

}

func rejectCall(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Access-Control-Allow-Headers", "PKey,CSRFToken")

	if r.Method != "POST" {
		w.WriteHeader(200)
		return
	}

	From := r.FormValue("From")

	if mysite.enable {
		token := r.Header.Get("CSRFtoken")

		uid, err := mysite.checkCRSF(token)
		if err != nil {
			log.Printf("API  mysite.checkCRSF(%s) \r\n", token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkCRSF API error", http.StatusForbidden)
			return
		}

		FromId, err := strconv.Atoi(From)

		if err != nil {
			http.Error(w, "bad From id", http.StatusForbidden)
			return
		}

		sendMsgClient(FromId, Message{messageType: 2, fromUID: uid})
	} else {

		frompub, err := pubKeyFromText(From, "hex")
		if err != nil {
			http.Error(w, "bad From", http.StatusForbidden)
			return
		}

		srcpub, err := pubKeyFromText(r.Header.Get("PKey"), "hex")
		if err != nil {
			http.Error(w, "bad PKey", http.StatusForbidden)
			return
		}

		Signature := r.FormValue("signature")

		sendMsgClientPkey(frompub, Message{messageType: 2, answer: Signature, fromUID: 0, fromPubKey: srcpub})
	}

}

func acceptCall(w http.ResponseWriter, r *http.Request) {
	var newCall *Call = nil
	var FromId, uid int
	var Signature string
	var mypub, frompub *ecdsa.PublicKey

	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Access-Control-Allow-Headers", "PKey,CSRFToken")

	if r.Method != "POST" {
		w.WriteHeader(200)
		return
	}

	From := r.FormValue("From")

	if mysite.enable {

		token := r.Header.Get("CSRFtoken")

		uid, err := mysite.checkCRSF(token)
		if err != nil {
			log.Printf("API  mysite.checkCRSF(%s) \r\n", token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkAppel API error", http.StatusForbidden)
			return
		}

		err = mysite.checkAppel(FromId, token)
		if err != nil {
			log.Printf("API  mysite.checkAppel(%d, %s) \r\n", FromId, token)
			log.Println("error ", err)
			http.Error(w, "mysite.checkAppel API error", http.StatusForbidden)
			return
		}

		FromId, err = strconv.Atoi(From)
		if err != nil {
			http.Error(w, "bad source id", http.StatusInternalServerError)
			return
		}

		newCall = findCall(FromId, uid)

	} else {
		var err error
		frompub, err = pubKeyFromText(From, "hex")
		if err != nil {
			http.Error(w, "bad From", http.StatusForbidden)
			return
		}
		mypub, err = pubKeyFromText(r.Header.Get("PKey"), "hex")
		if err != nil {
			http.Error(w, "bad pkey", http.StatusForbidden)
			return
		}

		Signature = r.FormValue("signature")

		newCall = findCallPKey(frompub, mypub)
	}

	if newCall == nil {
		//create new room

		sampleRate := 48000
		channels := 1
		latencyMS := 100
		if mysite.enable {

			sendMsgClient(FromId, Message{messageType: 3, fromUID: uid, fromPubKey: nil})
			newCall = &Call{from: FromId, to: uid, output: outputChannel{sampleRate: sampleRate, channels: channels, latencyMS: latencyMS, buffSize: (latencyMS * sampleRate * channels * 2) / 1000}}

		} else {

			sendMsgClientPkey(frompub, Message{messageType: 3, answer: Signature, fromUID: 0, fromPubKey: mypub})
			newCall = &Call{fromPKEY: frompub, toPKEY: mypub, output: outputChannel{sampleRate: sampleRate, channels: channels, latencyMS: latencyMS, buffSize: (latencyMS * sampleRate * channels * 2) / 1000}}

		}

		newCall.ticker = time.NewTicker(time.Millisecond * time.Duration(latencyMS))

		callsMut.Lock()
		callsList = append(callsList, newCall)
		callsMut.Unlock()
	}

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
