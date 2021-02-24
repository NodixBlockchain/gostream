package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-audio/riff"
	"github.com/gorilla/websocket"
)

type inputChannelBuffer struct {
	buffer []byte
	nRead  int
}

type inputChannel struct {
	sampleRate int
	channels   int
	buffers    []inputChannelBuffer
	totalRead  int
	startTime  time.Time
}

type roomClient struct {
	clientConn http.ResponseWriter
	channel    chan []int16
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
	inputs   []inputChannel
	output   outputChannel

	clients    []roomClient
	clientsMut sync.Mutex
}

var roomList []Room

// wsHandler implements the Handler Interface
type wsHandler struct{}

type Encoder struct {
	w            http.ResponseWriter
	SampleRate   int
	BitDepth     int
	NumChans     int
	WrittenBytes int
}

func NewEncoder(w http.ResponseWriter, outChan outputChannel) *Encoder {
	return &Encoder{
		w:            w,
		SampleRate:   outChan.sampleRate,
		BitDepth:     16,
		NumChans:     outChan.channels,
		WrittenBytes: 0}
}

func (e *Encoder) AddLE(src interface{}) error {
	e.WrittenBytes += binary.Size(src)
	return binary.Write(e.w, binary.LittleEndian, src)
}

func (e *Encoder) writeHeader() error {

	lenAudio := 0x1FFFFFFF

	lenFile := lenAudio + 44

	// riff ID
	if err := e.AddLE(riff.RiffID); err != nil {
		return err
	}
	// file size uint32, to update later on.
	if err := e.AddLE(uint32(lenFile - 8)); err != nil {
		return err
	}
	// wave headers
	if err := e.AddLE(riff.WavFormatID); err != nil {
		return err
	}
	// form
	if err := e.AddLE(riff.FmtID); err != nil {
		return err
	}
	// chunk size
	if err := e.AddLE(uint32(16)); err != nil {
		return err
	}
	// wave format
	if err := e.AddLE(uint16(1)); err != nil {
		return err
	}
	// num channels
	if err := e.AddLE(uint16(e.NumChans)); err != nil {
		return fmt.Errorf("error encoding the number of channels - %v", err)
	}
	// samplerate
	if err := e.AddLE(uint32(e.SampleRate)); err != nil {
		return fmt.Errorf("error encoding the sample rate - %v", err)
	}
	blockAlign := e.NumChans * e.BitDepth / 8
	// avg bytes per sec
	if err := e.AddLE(uint32(e.SampleRate * blockAlign)); err != nil {
		return fmt.Errorf("error encoding the avg bytes per sec - %v", err)
	}
	// block align
	if err := e.AddLE(uint16(blockAlign)); err != nil {
		return err
	}
	// bits per sample
	if err := e.AddLE(uint16(e.BitDepth)); err != nil {
		return fmt.Errorf("error encoding bits per sample - %v", err)
	}

	// sound header
	if err := e.AddLE(riff.DataFormatID); err != nil {
		return fmt.Errorf("error encoding sound header %v", err)
	}

	// write a chunksize
	if err := e.AddLE(uint32(lenAudio)); err != nil {
		return fmt.Errorf("%v when writing wav data chunk size header", err)
	}

	return nil
}

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

	//w.Write([]byte("HTTP 200 OK\r\ncontent-type:audio/wav\r\nContent-Length: 2400000000\r\n\r\n"))

	w.Header().Set("content-type", "audio/wav")
	w.Header().Set("content-Length", "2400000000")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(200)

	/*
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
			return
		}

		conn, bufrw, err := hj.Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	*/

	// Don't forget to close the connection:
	//defer conn.Close()

	var newClient roomClient
	newClient.channel = make(chan []int16, 1)
	newClient.clientConn = w

	roomList[roomFound].clientsMut.Lock()
	newClientIndex := len(roomList[roomFound].clients)
	roomList[roomFound].clients = append(roomList[roomFound].clients, newClient)
	roomList[roomFound].clientsMut.Unlock()

	log.Println("new client : ", len(roomList[0].clients))

	e := NewEncoder(roomList[roomFound].clients[newClientIndex].clientConn, roomList[roomFound].output)
	err = e.writeHeader()
	if err != nil {
		http.Error(w, "error writing wav header  ", http.StatusInternalServerError)
		return
	}

	for {
		newBuffer := <-roomList[roomFound].clients[newClientIndex].channel
		err = binary.Write(w, binary.LittleEndian, newBuffer)
		if err != nil {
			//log.Println("write error ", err)
		}
	}

}

func (wsh wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	var err error
	var roomID int
	var qRoom *Room
	var myInputId int

	roomID, err = strconv.Atoi(r.FormValue("roomID"))

	for i := 0; i < len(roomList); i++ {

		if roomList[i].id == roomID {

			qRoom = &roomList[i]
			break
		}
	}

	log.Printf("Hello, %s \n", qRoom.name)

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

	var newChannel inputChannel
	newChannel.sampleRate = 48000
	newChannel.channels = 1
	newChannel.totalRead = 0

	myInputId = len(qRoom.inputs)
	qRoom.inputs = append(qRoom.inputs, newChannel)

	newChannel.startTime = time.Now()

	for {
		_, message, err := wsConn.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}

		qRoom.inputs[myInputId].totalRead += len(message)

		//timeDiff := (time.Now().UnixNano() - newChannel.startTime.UnixNano()) / 1000000
		//log.Printf("stat read : %d %d %d %d ", mt, len(message), timeDiff, int64(qRoom.inputs[myInputId].totalRead)/(timeDiff))
		//log.Println("new input", message[0:16])

		var newBuffer inputChannelBuffer
		newBuffer.buffer = message
		newBuffer.nRead = 0

		qRoom.inputs[myInputId].buffers = append(qRoom.inputs[myInputId].buffers, newBuffer)

	}

}

func mixOutputChannel(inputs []inputChannel, nSamples int) []int16 {

	newBuffer := make([]int16, nSamples, nSamples)

	for i := 0; i < len(inputs); i++ {

		myinput := &inputs[i]

		nWriteChan := 0

		log.Printf("out %d \n", nSamples)

		for nWriteChan < nSamples {

			if len(myinput.buffers) > 0 {

				var curBuffer = &myinput.buffers[0]

				inputSample := int16(curBuffer.buffer[curBuffer.nRead]) + (int16(curBuffer.buffer[curBuffer.nRead+1]) << 8)
				curBuffer.nRead += 2

				newBuffer[nWriteChan] = inputSample

				if (curBuffer.nRead + 1) >= len(curBuffer.buffer) {
					myinput.buffers = myinput.buffers[1:]
				}
			} else {
				//log.Println("starvation")
				newBuffer[nWriteChan] = 0
			}

			nWriteChan++
		}
	}

	return newBuffer
}

func mixloop(t time.Time, room *Room) {

	newBuffer := mixOutputChannel(room.inputs, room.output.buffSize/2)

	for nClient := 0; nClient < len(room.clients); nClient++ {
		room.clients[nClient].channel <- newBuffer
	}
}

func createOutputChannel(sampleRate int, channels int, latencyMS int) outputChannel {
	var newOutput outputChannel
	newOutput.sampleRate = sampleRate
	newOutput.channels = channels
	newOutput.latencyMS = latencyMS
	newOutput.buffSize = (newOutput.latencyMS * newOutput.sampleRate * newOutput.channels * 2) / 1000
	return newOutput
}

func main() {

	var myRoom Room

	myRoom.id = 1
	myRoom.desc = ""
	myRoom.name = "my room"
	myRoom.RoomType = ""
	myRoom.output = createOutputChannel(48000, 1, 100)
	myRoom.inputs = make([]inputChannel, 0, 128)

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

	http.HandleFunc("/joinRoom", handleJoinRoom)
	log.Fatal(http.ListenAndServe(":8081", nil))

}
