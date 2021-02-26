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

type inputChannelBuffer struct {
	buffer []byte
	nRead  int
}

type inputChannel struct {
	id         int
	sampleRate int
	channels   int
	buffers    []*inputChannelBuffer
	totalRead  int
	startTime  time.Time
}

type roomClient struct {
	id         int
	clientConn http.ResponseWriter
	channel    chan []int16
}

type clientBuffer struct {
	clientid int
	buffer   []int16
}

type outputChannel struct {
	sampleRate int
	channels   int
	latencyMS  int
	buffSize   int
}

type Room struct {
	id       int
	name     string
	desc     string
	RoomType string

	currentInputId  int
	currentclientId int

	inputs     []*inputChannel
	inputMut   sync.Mutex
	clients    []*roomClient
	clientsMut sync.Mutex

	output outputChannel
}

var roomList []Room

func (r *Room) removeInput(idx int) {
	r.inputMut.Lock()

	r.inputs[idx] = r.inputs[len(r.inputs)-1]
	r.inputs = r.inputs[:len(r.inputs)-1]

	r.inputMut.Unlock()
}

func (r *Room) getInput(inputId int) *inputChannel {

	var myinput *inputChannel = nil

	r.inputMut.Lock()

	for i := 0; i < len(r.inputs); i++ {

		if r.inputs[i].id == inputId {
			myinput = r.inputs[i]
			break
		}
	}

	r.inputMut.Unlock()

	return myinput
}

func (r *Room) addInput(sampleRate int, chans int) int {

	r.inputMut.Lock()

	newinputId := r.currentInputId
	r.inputs = append(r.inputs, &inputChannel{id: newinputId, sampleRate: sampleRate, channels: chans, totalRead: 0, startTime: time.Now()})
	r.currentInputId++

	r.inputMut.Unlock()

	return newinputId
}
func (r *Room) addClient(w http.ResponseWriter) int {

	r.clientsMut.Lock()

	newClientid := r.currentclientId
	r.clients = append(r.clients, &roomClient{id: newClientid, channel: make(chan []int16, 2), clientConn: w})
	r.currentclientId++

	r.clientsMut.Unlock()

	return newClientid
}

func (r *Room) writeClientChannel(buf clientBuffer) error {

	r.clientsMut.Lock()

	for i := 0; i < len(r.clients); i++ {

		if r.clients[i].id == buf.clientid {
			r.clients[i].channel <- buf.buffer
			break
		}
	}

	r.clientsMut.Unlock()

	return nil
}

func (r *Room) getClient(clientId int) *roomClient {

	var myclient *roomClient = nil

	r.clientsMut.Lock()

	for i := 0; i < len(r.clients); i++ {

		if r.clients[i].id == clientId {
			myclient = r.clients[i]
			break
		}
	}

	r.clientsMut.Unlock()

	return myclient
}

func (r *Room) getClientBuffers() []clientBuffer {
	var buffers []clientBuffer

	r.clientsMut.Lock()
	buffers = make([]clientBuffer, len(r.clients), len(r.clients))

	for i, client := range r.clients {
		buffers[i].buffer = make([]int16, r.output.buffSize/2, r.output.buffSize/2)
		buffers[i].clientid = client.id
	}
	r.clientsMut.Unlock()

	return buffers
}

func (r *Room) removeClient(idx int) {
	r.clientsMut.Lock()

	r.clients[idx] = r.clients[len(r.clients)-1]
	r.clients = r.clients[:len(r.clients)-1]

	r.clientsMut.Unlock()
}

func (r *Room) createOutputChannel(sampleRate int, channels int, latencyMS int) error {

	r.output = outputChannel{sampleRate: sampleRate, channels: channels, latencyMS: latencyMS, buffSize: (latencyMS * sampleRate * channels * 2) / 1000}
	return nil
}

func (r *Room) mixOutputChannel() []clientBuffer {

	nSamples := r.output.buffSize / 2

	//create output buffers for each clients
	clientBuffers := r.getClientBuffers()

	log.Printf("out %d \n", nSamples)

	//iterate through room inputs
	for _, myinput := range r.inputs {

		if len(myinput.buffers) > 0 {

			var curBuffer = myinput.buffers[0]

			//iterate through room input samples to fill the buffer
			for nWriteChan := 0; nWriteChan < nSamples; nWriteChan++ {

				//read sample from input
				inputSample := int16(curBuffer.buffer[curBuffer.nRead]) + (int16(curBuffer.buffer[curBuffer.nRead+1]) << 8)
				curBuffer.nRead += 2

				//unshift first buffer when all data is read
				if (curBuffer.nRead + 1) >= len(curBuffer.buffer) {

					myinput.buffers = myinput.buffers[1:]

					//break input loop if no more buffers
					if len(myinput.buffers) <= 0 {
						//log.Println("starvation")
						break
					}
					//update pointer to first buffer
					curBuffer = myinput.buffers[0]
				}

				//iterate through room client buffers to mix the sample
				for _, buffer := range clientBuffers {

					var newsample int32 = int32(buffer.buffer[nWriteChan]) + int32(inputSample)

					if newsample > 32767 {
						buffer.buffer[nWriteChan] = 32767
					} else if newsample < -32768 {
						buffer.buffer[nWriteChan] = -32768
					} else {
						buffer.buffer[nWriteChan] = int16(newsample)
					}

				}
			}
		}
	}
	return clientBuffers
}

func mixloop(t time.Time, room *Room) {

	var buffers = room.mixOutputChannel()

	for _, mybuf := range buffers {
		room.writeClientChannel(mybuf)
	}

}

// wsHandler implements the Handler Interface
type wsHandler struct{}

func handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	var err error
	var roomID int

	roomID, err = strconv.Atoi(r.FormValue("roomID"))

	if err != nil {
		http.Error(w, "bad room id", http.StatusInternalServerError)
		return
	}

	roomFound := -1

	for i := 0; i < len(roomList); i++ {

		if roomList[i].id == roomID {

			roomFound = i
			break
		}
	}

	if roomFound == -1 {
		http.Error(w, "room not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("content-type", "audio/ogg")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(200)

	newClientId := roomList[roomFound].addClient(w)

	log.Println("new client : ", newClientId)

	client := roomList[roomFound].getClient(newClientId)

	e := getEncoder("opus", w, roomList[roomFound].output)

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

	roomList[roomFound].removeClient(newClientId)

}

func (wsh wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	var err error
	var roomID int
	var myInputId int

	roomID, err = strconv.Atoi(r.FormValue("roomID"))

	if err != nil {
		http.Error(w, "not room id", http.StatusInternalServerError)
		return
	}

	roomFound := -1

	for i := 0; i < len(roomList); i++ {

		if roomList[i].id == roomID {

			roomFound = i
			break
		}
	}

	if roomFound == -1 {
		http.Error(w, "room not found", http.StatusInternalServerError)
		return
	}

	log.Printf("new audio input in %s \n", roomList[roomFound].name)

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

	myInputId = roomList[roomFound].addInput(48000, 1)
	myinput := roomList[roomFound].getInput(myInputId)

	if myinput == nil {

		http.Error(w, "unable to add new input to room", http.StatusInternalServerError)
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

	roomList[roomFound].removeInput(myInputId)

}

func main() {

	var myRoom Room

	myRoom.id = 1
	myRoom.desc = ""
	myRoom.name = "my room"
	myRoom.RoomType = ""

	myRoom.createOutputChannel(48000, 1, 100)

	myRoom.inputs = make([]*inputChannel, 0, 128)

	roomList = append(roomList, myRoom)

	ticker := time.NewTicker(time.Millisecond * time.Duration(myRoom.output.latencyMS))
	go func() {
		for t := range ticker.C {
			//Call the periodic function here.
			mixloop(t, &roomList[0])
		}
	}()

	fmt.Println("Hello, World!")

	router := http.NewServeMux()
	router.Handle("/joinRoom", wsHandler{}) //handels websocket connections

	go func() {
		log.Fatal(http.ListenAndServe(":8080", router))
	}()

	http.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("./js"))))
	http.Handle("/html/", http.StripPrefix("/html/", http.FileServer(http.Dir("./html"))))
	http.HandleFunc("/joinRoom", handleJoinRoom)
	log.Fatal(http.ListenAndServe(":8081", nil))

}
