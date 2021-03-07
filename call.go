package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
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

var challenges map[string]string
var clientChallenges map[string]string

var challengesMut sync.Mutex

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

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		var rnds []byte

		rnds = make([]byte, 1, 1)

		n, e := rand.Read(rnds)
		if (n != 1) || (e != nil) {
			log.Println("error rand")
			return ""
		}

		c := int(rnds[0]) % (len(letterRunes) - 1)

		b[i] = letterRunes[c]
	}
	return string(b)
}

func hashPubkey(srcpub *ecdsa.PublicKey) string {
	h := md5.New()
	h.Write(elliptic.Marshal(privateKey.Curve, srcpub.X, srcpub.Y))
	hh := h.Sum(nil)
	myh := make([]byte, hex.EncodedLen(len(hh)))
	hex.Encode(myh, hh)

	return string(myh)
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

	mypubHEX := hex.EncodeToString(elliptic.Marshal(privateKey.Curve, privateKey.PublicKey.X, privateKey.PublicKey.Y))

	w.Write([]byte("event: pubkey\ndata:" + mypubHEX + "\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	for {
		message := <-newMessageClient.channel

		var messageBody, messageBodyText string
		var messageHeader string

		if message.messageType == 1 {
			messageHeader = "event: newCall\ndata: "
		}
		if message.messageType == 2 {
			messageHeader = "event: declineCall\ndata: "
		}
		if message.messageType == 3 {
			messageHeader = "event: acceptedCall\ndata: "
		}
		if message.messageType == 4 {
			messageHeader = "event: answer\ndata: "
		}
		if message.messageType == 5 {
			messageHeader = "event: answer2\ndata: "
		}

		if message.fromUID != 0 {
			messageBody += "{ \"from\": " + strconv.Itoa(message.fromUID) + "}"

		} else {

			messageBodyText = "{"

			if message.messageType == 1 {
				messageBodyText += "\"challenge\": \"" + message.challenge + "\","
			}
			if message.messageType == 2 || message.messageType == 3 {
				messageBodyText += "\"answer\": \"" + message.answer + "\","
			}
			if message.messageType == 4 {
				messageBodyText += "\"challenge\": \"" + message.challenge + "\","
				messageBodyText += "\"answer\": \"" + message.answer + "\","
			}
			if message.messageType == 5 {
				messageBodyText += "\"challenge\": \"" + message.challenge + "\","
				messageBodyText += "\"answer\": \"" + message.answer + "\","
			}

			messageBodyText += "\"from\": \"" + hex.EncodeToString(elliptic.MarshalCompressed(message.fromPubKey.Curve, message.fromPubKey.X, message.fromPubKey.Y)) + "\""
			messageBodyText += "}"

			var messageBodyHex []byte

			messageBodyHex = make([]byte, hex.EncodedLen(len(messageBodyText)), hex.EncodedLen(len(messageBodyText)))

			hex.Encode(messageBodyHex, []byte(messageBodyText))

			messageBody = "\"" + string(messageBodyHex) + "\""

		}

		nWrote, err := w.Write([]byte(messageHeader))
		if (err != nil) || (nWrote < len(messageHeader)) {
			break
		}

		nWrote, err = w.Write([]byte(messageBody + "\n\n"))
		if (err != nil) || (nWrote < len(messageBody+"\n\n")) {
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

func getCallTicket(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Access-Control-Allow-Headers", "PKey,CSRFToken")

	if r.Method != "GET" {
		w.WriteHeader(200)
		return
	}

	srcpub, err := pubKeyFromText(r.Header.Get("PKey"), "hex")
	if err != nil {
		http.Error(w, "bad PKey", http.StatusForbidden)
		return
	}
	Challenge := RandStringRunes(8)

	challengesMut.Lock()
	challenges[string(hashPubkey(srcpub))] = Challenge
	challengesMut.Unlock()

	w.Write([]byte(Challenge))

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

		srcpub, err := pubKeyFromText(r.Header.Get("PKey"), "hex")
		if err != nil {
			http.Error(w, "bad PKey", http.StatusForbidden)
			return
		}

		Signature, err := hex.DecodeString(r.FormValue("signature"))

		challengesMut.Lock()
		res := ecdsa.VerifyASN1(srcpub, []byte(challenges[hashPubkey(srcpub)]), Signature)
		challengesMut.Unlock()
		if !res {
			http.Error(w, "bad signature", http.StatusForbidden)
			return
		}

		dstpub, err := pubKeyFromText(Destination, "hex")
		if err != nil {
			http.Error(w, "bad Destination", http.StatusForbidden)
			return
		}

		Challenge := r.FormValue("challenge")

		challengesMut.Lock()
		clientChallenges[hashPubkey(dstpub)] = Challenge
		challengesMut.Unlock()

		sendMsgClientPkey(dstpub, Message{messageType: 1, challenge: Challenge, fromUID: 0, fromPubKey: srcpub})

		newChallenge := RandStringRunes(8)

		challenges[hashPubkey(srcpub)] = newChallenge

		w.Write([]byte(newChallenge))
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

	Sign, err := hex.DecodeString(Signature)
	if err != nil {
		http.Error(w, "bad signature format", http.StatusForbidden)
		return
	}

	challengesMut.Lock()
	res := ecdsa.VerifyASN1(srcpub, []byte(clientChallenges[hashPubkey(srcpub)]), Sign)
	challengesMut.Unlock()
	if !res {
		http.Error(w, "bad answer signature", http.StatusForbidden)
		return
	}

	Challenge := r.FormValue("challenge")

	challengesMut.Lock()
	clientChallenges[hashPubkey(dstpub)] = Challenge
	challengesMut.Unlock()

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

	Sign, err := hex.DecodeString(Signature)
	if err != nil {
		http.Error(w, "bad signature format", http.StatusForbidden)
		return
	}

	challengesMut.Lock()
	res := ecdsa.VerifyASN1(srcpub, []byte(clientChallenges[hashPubkey(srcpub)]), Sign)
	challengesMut.Unlock()
	if !res {
		http.Error(w, "bad answer2 signature", http.StatusForbidden)
		return
	}

	Challenge := r.FormValue("challenge")

	challengesMut.Lock()
	clientChallenges[hashPubkey(dstpub)] = Challenge
	challengesMut.Unlock()

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

		Sign, err := hex.DecodeString(Signature)
		if err != nil {
			http.Error(w, "bad signature format", http.StatusForbidden)
			return
		}

		challengesMut.Lock()
		res := ecdsa.VerifyASN1(srcpub, []byte(clientChallenges[hashPubkey(srcpub)]), Sign)
		challengesMut.Unlock()
		if !res {
			http.Error(w, "bad rejectCall signature", http.StatusForbidden)
			return
		}

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
		var err error

		token := r.Header.Get("CSRFtoken")

		uid, err = mysite.checkCRSF(token)
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

		Sign, err := hex.DecodeString(Signature)
		if err != nil {
			http.Error(w, "bad signature format", http.StatusForbidden)
			return
		}

		challengesMut.Lock()
		res := ecdsa.VerifyASN1(mypub, []byte(clientChallenges[hashPubkey(mypub)]), Sign)
		challengesMut.Unlock()
		if !res {
			http.Error(w, "bad acceptcall signature", http.StatusForbidden)
			return
		}

		newCall = findCallPKey(frompub, mypub)
	}

	if newCall == nil {
		//create new room

		sampleRate := 48000
		channels := 1
		latencyMS := 100
		if mysite.enable {
			newCall = &Call{from: FromId, to: uid, output: outputChannel{sampleRate: sampleRate, channels: channels, latencyMS: latencyMS, buffSize: (latencyMS * sampleRate * channels * 2) / 1000}}

		} else {
			newCall = &Call{fromPKEY: frompub, toPKEY: mypub, output: outputChannel{sampleRate: sampleRate, channels: channels, latencyMS: latencyMS, buffSize: (latencyMS * sampleRate * channels * 2) / 1000}}
		}

		newCall.ticker = time.NewTicker(time.Millisecond * time.Duration(latencyMS))

		callsMut.Lock()
		callsList = append(callsList, newCall)
		callsMut.Unlock()
	}

	if mysite.enable {
		sendMsgClient(FromId, Message{messageType: 3, answer: Signature, fromUID: uid, fromPubKey: nil})
	} else {
		sendMsgClientPkey(frompub, Message{messageType: 3, answer: Signature, fromUID: 0, fromPubKey: mypub})

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
