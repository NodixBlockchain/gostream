package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
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

var privateKey *ecdsa.PrivateKey

var mysite site = site{siteURL: "http://172.16.230.1", siteOrigin: "http://172.16.230.1", enable: true}

//var mysite site = site{siteURL: "http://localhost", siteOrigin: "http://localhost", enable: false}

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

	newRoom = &Room{id: roomId, name: "my room", desc: "", RoomType: "", callFrom: 0, callTo: 0, inputs: make([]*inputChannel, 0, 128), output: outputChannel{sampleRate: sampleRate, channels: channels, latencyMS: latencyMS, buffSize: (latencyMS * sampleRate * channels * 2) / 1000}}
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
	var clientID int
	var token string

	w.Header().Set("Access-Control-Allow-Origin", mysite.siteOrigin)
	w.Header().Set("Access-Control-Allow-Headers", "PKey,CSRFToken")

	if r.Method != "GET" {
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

		otherID, err := strconv.Atoi(otherSTR)
		if err != nil {
			http.Error(w, "bad other id", http.StatusInternalServerError)
			return
		}

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

		log.Printf("new client : %d in call [%d-%d] using token '%s'", clientID, userid, otherID, token)

	} else {

		otherpub, err := pubKeyFromText(otherSTR, "hex")
		if err != nil {
			http.Error(w, "bad From", http.StatusForbidden)
			return
		}

		mypub, err := pubKeyFromText(r.Header.Get("PKey"), "hex")
		if err != nil {
			http.Error(w, "bad PKey", http.StatusForbidden)
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

		log.Printf("new client : %d in call [%x-%x]", clientID, mypub, otherpub)
	}

	if call.clients[clientID] != nil {
		http.Error(w, "user already connected", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(200)

	call.clients[clientID] = &roomClient{id: clientID, token: token, channel: make(chan []int16, 1), clientConn: w}

	e := getEncoder(format, w, call.output)
	err = e.writeHeader()

	if err != nil {
		http.Error(w, "error initializing audio encoder  ", http.StatusInternalServerError)
		return
	}

	for {
		newBuffer := <-call.clients[clientID].channel
		if e.writeBuffer(newBuffer) != nil {
			break
		}
	}

	call.clients[clientID] = nil

}

// wsHandler implements the Handler Interface
type wsCallHandler struct{}

func (wsh wsCallHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var inputID int
	var err error
	var token string
	var call *Call

	if mysite.enable {

		token = r.FormValue("token")

		otherID, err := strconv.Atoi(r.FormValue("otherID"))
		if err != nil {
			http.Error(w, "no other id", http.StatusInternalServerError)
			return
		}

		userid, err := mysite.checkCRSF(token)
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

		log.Printf("new audio input %d in call [%d-%d] using token '%s'\n", inputID, call.from, call.to, token)

	} else {

		otherpub, err := pubKeyFromText(r.FormValue("otherID"), "hex")
		if err != nil {
			http.Error(w, "bad From", http.StatusForbidden)
			return
		}

		mypub, err := pubKeyFromText(r.FormValue("PKey"), "hex")
		if err != nil {
			http.Error(w, "bad PKey", http.StatusForbidden)
			return
		}

		call = findCallPKey(mypub, otherpub)
		if call.fromPKEY.Equal(mypub) {
			inputID = 0
		} else {
			inputID = 1
		}

		log.Printf("new audio input %d in call [%x-%x] \n", inputID, mypub, otherpub)

	}

	if call.inputs[inputID] != nil {
		http.Error(w, "user already connected", http.StatusInternalServerError)
		return
	}

	call.inputs[inputID] = &inputChannel{id: inputID, token: token, sampleRate: 48000, channels: 1, totalRead: 0, startTime: time.Now()}

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

		call.inputs[inputID].totalRead += len(message)

		call.inputs[inputID].bufMut.Lock()
		call.inputs[inputID].buffers = append(call.inputs[inputID].buffers, &inputChannelBuffer{buffer: message, nRead: 0})
		call.inputs[inputID].bufMut.Unlock()
	}

	call.inputs[inputID] = nil
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
var tokens map[string]int

var userid = 1

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

func newCRSF(w http.ResponseWriter, r *http.Request) {

	newtoken := RandStringRunes(12)

	tokens[newtoken] = userid
	userid++

	w.Header().Set("content-type", "application/json")
	w.Write([]byte("{\"token\" : \"" + newtoken + "\"}"))
}

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

	/*
		tokens = make(map[string]int)

		routerSite := mux.NewRouter()

		routerSite.HandleFunc("/Membres/keyXCHG", keyXCHG)
		routerSite.HandleFunc("/Membres/newCRSF", newCRSF)
		routerSite.HandleFunc("/Membres/crossLogin/{token}", crossLogin)

		routerSite.HandleFunc("/Membres/peuxAppeller/{destination:[0-9]+}/{token:[a-zA-Z0-9]+}", peuxAppeller)

		routerSite.HandleFunc("/Groupes/envoieAudioGroup/{roomid:[0-9]+}/{token:[a-zA-Z0-9]+}/{on:[0-9]+}", envoieAudioGroup)
		routerSite.HandleFunc("/Groupes/ecouteAudioGroup/{roomid:[0-9]+}/{token:[a-zA-Z0-9]+}/{on:[0-9]+}", ecouteAudioGroup)

		routerSite.Handle("/js/{file}", http.StripPrefix("/js/", http.FileServer(http.Dir("./js"))))
		routerSite.Handle("/html/{file}", http.StripPrefix("/html/", http.FileServer(http.Dir("./html"))))

		go func() {
			log.Fatal(http.ListenAndServe(":80", routerSite))
		}()
	*/

	router := http.NewServeMux()
	router.Handle("/upRoom", wsHandler{})     //handels websocket connections
	router.Handle("/upCall", wsCallHandler{}) //handels websocket connections

	router.HandleFunc("/joinRoom", handleJoinRoom)
	router.HandleFunc("/joinCall", handleJoinCall)

	router.HandleFunc("/newCall", newCall)
	router.HandleFunc("/answer", answer)
	router.HandleFunc("/answer2", answer2)
	router.HandleFunc("/rejectCall", rejectCall)
	router.HandleFunc("/acceptCall", acceptCall)
	router.HandleFunc("/messages", messages)

	router.HandleFunc("/tokenCheck", tokenCheck)
	router.HandleFunc("/keyXCHG", keyXCHG)

	router.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("./js"))))
	router.Handle("/html/", http.StripPrefix("/html/", http.FileServer(http.Dir("./html"))))

	log.Fatal(http.ListenAndServe(":8080", router))

}
