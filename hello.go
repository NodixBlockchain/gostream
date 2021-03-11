package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//<\?php[' ']+echo[' ']+site_url\(["'"]([^']*)["'"][^)]*\);?[' ']+\?>
//<\?php[' ']+echo[' ']+base_url\(["'"]([^']*)["'"][^)]*\);?[' ']+\?>
//<\?php[' ']+echo[' ']+\$[^\[]*\[([^;]*)\];?[' ']*\?>

type Message struct {
	messageType int

	challenge         string
	answer            string
	audioOut, audioIn int

	fromUID    int
	fromPubKey *ecdsa.PublicKey
}

type messageClient struct {
	channel chan Message
	w       http.ResponseWriter

	userID int
	pubKey *ecdsa.PublicKey
}

var mysite site = site{siteURL: "http://172.16.230.1", siteOrigin: "http://172.16.230.1", enable: false}

//var mysite site = site{siteURL: "http://localhost", siteOrigin: "http://localhost", enable: true}

var sslCERT string = "/home/gostream/gostream.crt"
var sslKEY string = "/home/gostream/gostream.key"

var roomList []*Room
var roomsMut sync.Mutex

var privateKey *ecdsa.PrivateKey

var messageClients []*messageClient
var msgClientsMut sync.Mutex

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

	mypubHEX := hex.EncodeToString(elliptic.Marshal(privateKey.Curve, privateKey.PublicKey.X, privateKey.PublicKey.Y))

	w.Write([]byte("event: pubkey\ndata:" + mypubHEX + "\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	for {
		message := <-newMessageClient.channel

		var messageBody string
		var messageHeader string

		switch message.messageType {
		case 1:
			messageHeader = "event: newCall\ndata: "
		case 2:
			messageHeader = "event: declineCall\ndata: "
		case 3:
			messageHeader = "event: acceptedCall\ndata: "
		case 4:
			messageHeader = "event: answer\ndata: "
		case 5:
			messageHeader = "event: answer2\ndata: "
		case 6:
			messageHeader = "event: setAudioConf\ndata: "
		}

		messageBody = "{"

		if message.fromUID != 0 {
			messageBody += "\"from\": " + strconv.Itoa(message.fromUID)

		} else {

			if message.messageType != 1 {
				messageBody += "\"answer\": \"" + message.answer + "\","
			}

			if message.messageType != 2 {
				messageBody += "\"challenge\": \"" + message.challenge + "\","
			}

			messageBody += "\"from\": \"" + hex.EncodeToString(elliptic.MarshalCompressed(message.fromPubKey.Curve, message.fromPubKey.X, message.fromPubKey.Y)) + "\""
		}

		if message.messageType == 6 {
			messageBody += ",\"in\": \"" + strconv.Itoa(message.audioIn) + "\""
			messageBody += ",\"out\": \"" + strconv.Itoa(message.audioOut) + "\""
		}

		messageBody += "}"

		nWrote, err := w.Write([]byte(messageHeader))
		if (err != nil) || (nWrote < len(messageHeader)) {
			break
		}
		if message.fromUID != 0 {
			nWrote, err = w.Write([]byte(messageBody + "\n\n"))
			if (err != nil) || (nWrote < len(messageBody+"\n\n")) {
				break
			}
		} else {

			messageBodyHex := make([]byte, hex.EncodedLen(len(messageBody)), hex.EncodedLen(len(messageBody)))

			hex.Encode(messageBodyHex, []byte(messageBody))

			nWrote, err = w.Write([]byte("\"" + string(messageBodyHex) + "\"\n\n"))
			if (err != nil) || (nWrote < len(messageBody+"\n\n")) {
				break
			}

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

			if !myroom.isActive() {
				continue
			}

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

	userid, err := mysite.checkCRSF(token)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("check CRSF Failed %s %v \r\n", token, err)))
		log.Printf("token %s check error %v ", token, err)
		return
	}
	w.Write([]byte(strconv.Itoa(userid)))
}

func keyXCHG(w http.ResponseWriter, r *http.Request) {

	pkeyb64 := r.FormValue("pubkey")
	//pubBytes, _ := base64.StdEncoding.DecodeString(pkeyb64)
	pubBytes, _ := hex.DecodeString(pkeyb64)

	signb64 := r.FormValue("sign")
	//sign, _ := base64.StdEncoding.DecodeString(signb64)
	sign, _ := hex.DecodeString(signb64)

	X, Y := elliptic.UnmarshalCompressed(privateKey.Curve, pubBytes)

	srcpub := &ecdsa.PublicKey{Curve: privateKey.Curve, X: X, Y: Y}

	if srcpub.X != nil {
		fmt.Printf("srcpub %x\n", srcpub)
	} else {
		return
	}

	var msg []byte = make([]byte, 11, 11)

	for i := 0; i < 11; i++ {
		msg[i] = byte(i)
	}

	msg[0] = 1

	res := ecdsa.VerifyASN1(srcpub, msg, sign)

	if !res {
		fmt.Printf("sign not check \n")
		return
	}

	fmt.Printf("sign check \n")

	var mypub ecdsa.PublicKey = privateKey.PublicKey

	fmt.Printf("mypub 1 %x\r\n", mypub)

	myk := elliptic.Marshal(mypub.Curve, mypub.X, mypub.Y)

	a, b := privateKey.Curve.ScalarMult(srcpub.X, srcpub.Y, privateKey.D.Bytes())

	fmt.Printf("derived key %x %x \r\n", a, b)

	w.Header().Set("content-type", "application/json")
	w.Write([]byte("{\"pubkey\" : \"" + hex.EncodeToString(myk) + "\"}"))

}

func handleJoinCall(w http.ResponseWriter, r *http.Request) {
	var err error
	var format string
	var otherSTR string
	var call *Call
	var otherID, userid, clientID int
	var token string
	var mypub, otherpub *ecdsa.PublicKey

	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Access-Control-Allow-Headers", "PKey,CSRFToken")

	if r.Method != "POST" {
		w.WriteHeader(200)
		return
	}

	format = r.FormValue("format")

	if len(format) <= 0 {
		format = "opus"
	}

	if format == "wav" {
		w.Header().Set("content-type", "audio/wav")
	} else {
		w.Header().Set("content-type", "audio/ogg")
	}

	otherSTR = r.FormValue("otherID")

	if mysite.enable {

		otherID, err = strconv.Atoi(otherSTR)
		if err != nil {
			http.Error(w, "bad other id", http.StatusInternalServerError)
			return
		}

		token = r.Header.Get("CSRFtoken")
		if token == "" {
			token = r.FormValue("token")
		}

		userid, err = mysite.checkCRSF(token)
		if err != nil {
			w.Write([]byte(fmt.Sprintf("check CRSF Failed %s %v \r\n", token, err)))
			log.Printf("token %s check error %v ", token, err)
			return
		}
		call = findCall(userid, otherID)

		if call == nil {
			http.Error(w, "call not found", http.StatusInternalServerError)
			return
		}

		if call.from == userid {
			clientID = 0
		} else {
			clientID = 1
		}

		if call.clients[clientID] != nil {
			http.Error(w, "user already connected", http.StatusInternalServerError)
			return
		}

		call.clients[clientID] = &roomClient{id: clientID, userID: userid, pubkey: nil, channel: make(chan []int16, 1), clientConn: w}

		log.Printf("new client : %d in call [%d-%d] using token '%s'", clientID, userid, otherID, token)

	} else {

		mypub, err = pubKeyFromText(r.Header.Get("PKey"), "hex")
		if err != nil {
			http.Error(w, "bad PKey", http.StatusForbidden)
			return
		}

		Signature, err := hex.DecodeString(r.FormValue("Signature"))
		if err != nil {
			http.Error(w, "bad signature", http.StatusForbidden)
			return
		}
		kh := hashPubkey(mypub)

		challengesMut.Lock()
		challenge := challenges[kh]
		challengesMut.Unlock()

		res := ecdsa.VerifyASN1(mypub, []byte(challenge), Signature)
		if !res {
			http.Error(w, "bad signature", http.StatusForbidden)
			return
		}

		otherpub, err = pubKeyFromText(otherSTR, "hex")
		if err != nil {
			http.Error(w, "bad From", http.StatusForbidden)
			return
		}

		call = findCallPKey(mypub, otherpub)

		if call == nil {
			http.Error(w, "call not found", http.StatusInternalServerError)
			return
		}

		if call.fromPKEY.Equal(mypub) {
			clientID = 0
		} else {
			clientID = 1
		}

		if call.clients[clientID] != nil {
			http.Error(w, "user already connected", http.StatusInternalServerError)
			return
		}

		call.clients[clientID] = &roomClient{id: clientID, userID: 0, pubkey: mypub, channel: make(chan []int16, 1), clientConn: w}

		log.Printf("new client : %d in call [%x-%x]", clientID, mypub, otherpub)
	}

	w.WriteHeader(200)

	e := getEncoder(format, w, call.output)
	err = e.writeHeader()

	if err != nil {
		http.Error(w, "error initializing audio encoder  ", http.StatusInternalServerError)
		return
	}

	if mysite.enable {
		call.updateAudioConf(userid)
	} else {
		call.updateAudioConfPKey(mypub)
	}

	for {
		newBuffer := <-call.clients[clientID].channel
		if e.writeBuffer(newBuffer) != nil {
			break
		}
	}

	call.clients[clientID] = nil

	if mysite.enable {
		call.updateAudioConf(userid)
	} else {
		call.updateAudioConfPKey(mypub)
	}

}

// wsHandler implements the Handler Interface
type wsCallHandler struct{}

func (wsh wsCallHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var inputID, otherID, userid int
	var err error
	var token string
	var call *Call
	var mypub, otherpub *ecdsa.PublicKey

	if mysite.enable {

		token = r.FormValue("token")

		otherID, err = strconv.Atoi(r.FormValue("otherID"))
		if err != nil {
			http.Error(w, "no other id", http.StatusInternalServerError)
			return
		}

		userid, err = mysite.checkCRSF(token)
		if err != nil {
			w.Write([]byte(fmt.Sprintf("check CRSF Failed %s %v \r\n", token, err)))
			log.Printf("token %s check error %v ", token, err)
			return
		}

		call = findCall(userid, otherID)

		if call == nil {
			http.Error(w, "call not found", http.StatusInternalServerError)
			return
		}

		if call.from == userid {
			inputID = 0
		} else {
			inputID = 1
		}

		if call.inputs[inputID] != nil {
			http.Error(w, "user already connected", http.StatusInternalServerError)
			return
		}

		call.inputs[inputID] = &inputChannel{id: inputID, userID: userid, sampleRate: 48000, channels: 1, totalRead: 0, startTime: time.Now()}

		log.Printf("new audio input %d in call [%d-%d] using token '%s'\n", inputID, call.from, call.to, token)

	} else {

		mypub, err = pubKeyFromText(r.FormValue("PKey"), "hex")
		if err != nil {
			http.Error(w, "bad PKey", http.StatusForbidden)
			return
		}

		Signature, err := hex.DecodeString(r.FormValue("Signature"))
		if err != nil {
			http.Error(w, "bad signature", http.StatusForbidden)
			return
		}

		kh := hashPubkey(mypub)

		challengesMut.Lock()
		challenge := challenges[kh]
		challengesMut.Unlock()

		res := ecdsa.VerifyASN1(mypub, []byte(challenge), Signature)
		if !res {
			http.Error(w, "bad signature", http.StatusForbidden)
			return
		}

		otherpub, err = pubKeyFromText(r.FormValue("otherID"), "hex")
		if err != nil {
			http.Error(w, "bad From", http.StatusForbidden)
			return
		}

		call = findCallPKey(mypub, otherpub)
		if call.fromPKEY.Equal(mypub) {
			inputID = 0
		} else {
			inputID = 1
		}

		if call.inputs[inputID] != nil {
			http.Error(w, "user already connected", http.StatusInternalServerError)
			return
		}

		call.inputs[inputID] = &inputChannel{id: inputID, pubkey: mypub, sampleRate: 48000, channels: 1, totalRead: 0, startTime: time.Now()}

		log.Printf("new audio input %d in call [%x-%x] \n", inputID, mypub, otherpub)

	}

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

	if mysite.enable {
		call.updateAudioConf(userid)
	} else {
		call.updateAudioConfPKey(mypub)
	}

	for {
		_, message, err := wsConn.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}

		call.inputs[inputID].totalRead += len(message)

		call.inputs[inputID].bufMut.Lock()
		call.inputs[inputID].buffers = append(call.inputs[inputID].buffers, &inputChannelBuffer{buffer: message, nRead: 0})
		call.inputs[inputID].bufMut.Unlock()
	}

	call.inputs[inputID] = nil

	if mysite.enable {
		call.updateAudioConf(userid)
	} else {
		call.updateAudioConfPKey(mypub)
	}
}

func handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	var err error
	var token, format string
	var newClientId, roomID int

	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Access-Control-Allow-Headers", "PKey,CSRFToken")

	if r.Method != "POST" {
		w.WriteHeader(200)
		return
	}

	roomID, err = strconv.Atoi(r.FormValue("roomID"))

	if err != nil {
		http.Error(w, "bad room id", http.StatusInternalServerError)
		return
	}

	room := grabRoom(roomID)

	if room == nil {
		http.Error(w, "room not found", http.StatusInternalServerError)
		return
	}

	format = r.FormValue("format")

	if len(format) <= 0 {
		format = "opus"
	}

	if mysite.enable {

		token = r.Header.Get("CSRFtoken")
		if token == "" {
			token = r.FormValue("token")
		}

		userid, err := mysite.checkCRSF(token)
		if err != nil {
			w.Write([]byte(fmt.Sprintf("check CRSF Failed %s %v \r\n", token, err)))
			log.Printf("token %s check error %v ", token, err)
			return
		}

		err = mysite.newListener(roomID, token, 1)
		if err != nil {
			log.Printf("API mysite.newListener(%d,%s,1) \r\n", roomID, token)
			log.Println("error ", err)

			http.Error(w, "mysite.newListener API error", http.StatusForbidden)
			return
		}

		newClientId = room.addClient(w, userid)

		log.Printf("new client : %d in room [%d] %s using token '%s'", newClientId, room.id, room.name, token)

	} else {

		mypub, err := pubKeyFromText(r.Header.Get("PKey"), "hex")
		if err != nil {
			http.Error(w, "bad PKey", http.StatusForbidden)
			return
		}

		Signature, err := hex.DecodeString(r.FormValue("Signature"))
		if err != nil {
			http.Error(w, "bad signature", http.StatusForbidden)
			return
		}

		kh := hashPubkey(mypub)

		challengesMut.Lock()
		challenge := challenges[kh]
		challengesMut.Unlock()

		res := ecdsa.VerifyASN1(mypub, []byte(challenge), Signature)
		if !res {
			http.Error(w, "bad signature", http.StatusForbidden)
			return
		}

		newClientId = room.addClientPKey(w, mypub)

		log.Printf("new client : %d in room [%d] %s using pubkey '%x'", newClientId, room.id, room.name, mypub)

	}

	if format == "wav" {
		w.Header().Set("content-type", "audio/wav")
	} else {
		w.Header().Set("content-type", "audio/ogg")
	}

	client := room.getClient(newClientId)

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
	var token string
	var myInputId int

	roomID, err := strconv.Atoi(r.FormValue("roomID"))

	if err != nil {
		http.Error(w, "not room id", http.StatusInternalServerError)
		return
	}

	room := grabRoom(roomID)

	if room == nil {
		http.Error(w, "room not found", http.StatusInternalServerError)
		return
	}

	if mysite.enable {

		token = r.FormValue("token")

		userid, err := mysite.checkCRSF(token)
		if err != nil {
			w.Write([]byte(fmt.Sprintf("check CRSF Failed %s %v \r\n", token, err)))
			log.Printf("token %s check error %v ", token, err)
			return
		}

		err = mysite.newInput(roomID, token, 1)
		if err != nil {
			log.Printf("API mysite.newInput(%d,%s,1) \r\n", roomID, token)
			log.Println("error : ", err)

			http.Error(w, "mysite.newInput API error", http.StatusForbidden)
			return
		}

		log.Printf("new audio input in %s using token '%s'\n", room.name, token)

		myInputId = room.addInput(48000, 1, userid)

	} else {

		mypub, err := pubKeyFromText(r.FormValue("PKey"), "hex")
		if err != nil {
			http.Error(w, "bad PKey", http.StatusForbidden)
			return
		}

		Signature, err := hex.DecodeString(r.FormValue("Signature"))
		if err != nil {
			http.Error(w, "bad signature", http.StatusForbidden)
			return
		}
		kh := hashPubkey(mypub)

		challengesMut.Lock()
		challenge := challenges[kh]
		challengesMut.Unlock()

		res := ecdsa.VerifyASN1(mypub, []byte(challenge), Signature)
		if !res {
			http.Error(w, "bad signature", http.StatusForbidden)
			return
		}

		log.Printf("new audio input in %s using pubkey '%x'\n", room.name, mypub)

		myInputId = room.addInputPKey(48000, 1, mypub)
	}

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

		myinput.bufMut.Lock()
		myinput.buffers = append(myinput.buffers, &inputChannelBuffer{buffer: message, nRead: 0})
		myinput.bufMut.Unlock()
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
var tokens map[string]int

var userid = 1

func crossLogin(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("content-type", "application/json")

	vars := mux.Vars(r)

	token := vars["token"]

	i, ok := tokens[token]

	if ok {
		w.Write([]byte(strconv.Itoa(i)))
	} else {
		w.Write([]byte("0"))
	}

}

func newCRSF(w http.ResponseWriter, r *http.Request) {

	newtoken := RandStringRunes(12)

	tokens[newtoken] = userid
	userid++

	w.Header().Set("content-type", "application/json")
	w.Write([]byte("{\"token\" : \"" + newtoken + "\"}"))
}

func envoieAudioGroup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")

	vars := mux.Vars(r)

	log.Printf("envoie audio room [%s] token  '%s', on[%s]\r\n", vars["roomid"], vars["token"], vars["on"])

	w.Write([]byte("1"))
}

func ecouteAudioGroup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")

	vars := mux.Vars(r)

	log.Printf("ecoute audio room [%s] token  '%s', on[%s]\r\n", vars["roomid"], vars["token"], vars["on"])

	//w.Write([]byte(r.URL.Path))
	w.Write([]byte("1"))
}

func peuxAppeller(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")

	vars := mux.Vars(r)

	log.Printf("peuxAppeller [%s] token '%s'\r\n", vars["destination"], vars["token"])

	//w.Write([]byte(r.URL.Path))
	w.Write([]byte("1"))
}
*/

func main() {

	fmt.Println("goStream starting !")

	var err error

	privateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	if err != nil {
		panic(err)
	}

	challenges = make(map[string]string)
	clientChallenges = make(map[string]string)

	/*
		tokens = make(map[string]int)

		routerSite := mux.NewRouter()

		routerSite.HandleFunc("/Membres/keyXCHG", keyXCHG)
		routerSite.HandleFunc("/Membres/newCRSF", newCRSF)
		routerSite.HandleFunc("/Membres/crossLogin/{token}", crossLogin)

		routerSite.HandleFunc("/Membres/peuxAppeller/{destination:[0-9]+}/{token:[a-zA-Z0-9]+}", peuxAppeller)

		routerSite.HandleFunc("/Groupes/envoieAudioGroup/{roomid:[0-9]+}/{token:[a-zA-Z0-9]+}/{on:[0-9]+}", envoieAudioGroup)
		routerSite.HandleFunc("/Groupes/ecouteAudioGroup/{roomid:[0-9]+}/{token:[a-zA-Z0-9]+}/{on:[0-9]+}", ecouteAudioGroup)

		routerSite.Handle("/js/{file}", http.StripPrefix("/js/", http.FileServer(http.Dir("./www/js"))))
		routerSite.Handle("/html/{file}", http.StripPrefix("/html/", http.FileServer(http.Dir("./www/html"))))

		go func() {
			log.Fatal(http.ListenAndServe(":80", routerSite))
		}()
	*/

	router := http.NewServeMux()
	router.Handle("/upRoom", wsHandler{})     //handels websocket connections
	router.Handle("/upCall", wsCallHandler{}) //handels websocket connections

	router.HandleFunc("/joinRoom", handleJoinRoom)
	router.HandleFunc("/joinCall", handleJoinCall)

	router.HandleFunc("/getCallTicket", getCallTicket)
	router.HandleFunc("/newCall", newCall)
	router.HandleFunc("/answer", answer)
	router.HandleFunc("/answer2", answer2)
	router.HandleFunc("/rejectCall", rejectCall)
	router.HandleFunc("/acceptCall", acceptCall)
	router.HandleFunc("/messages", messages)

	router.HandleFunc("/tokenCheck", tokenCheck)
	router.HandleFunc("/keyXCHG", keyXCHG)

	router.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("./www/js"))))
	router.Handle("/html/", http.StripPrefix("/html/", http.FileServer(http.Dir("./www/html"))))

	if _, err := os.Stat(sslCERT); !os.IsNotExist(err) {
		log.Fatal(http.ListenAndServeTLS(":8080", sslCERT, sslKEY, router))
	}

	log.Fatal(http.ListenAndServe(":8080", router))

}
