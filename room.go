package main

import (
	"net/http"
	"sync"
	"time"
)

type inputChannelBuffer struct {
	buffer []byte
	nRead  int
	time   time.Time
}

type inputChannel struct {
	id         int
	sampleRate int
	channels   int
	buffers    []*inputChannelBuffer
	totalRead  int
	startTime  time.Time
	token      string
	bufMut     sync.Mutex
}

type roomClient struct {
	id         int
	clientConn http.ResponseWriter
	channel    chan []int16
	token      string
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

	callFrom int
	callTo   int

	currentInputId  int
	currentclientId int

	inputs     []*inputChannel
	inputMut   sync.Mutex
	clients    []*roomClient
	clientsMut sync.Mutex

	output outputChannel
	ticker *time.Ticker
}

func (r *Room) addInput(sampleRate int, chans int, token string) int {

	r.inputMut.Lock()

	newinputId := r.currentInputId
	r.inputs = append(r.inputs, &inputChannel{id: newinputId, token: token, sampleRate: sampleRate, channels: chans, totalRead: 0, startTime: time.Now()})
	r.currentInputId++

	r.inputMut.Unlock()

	return newinputId
}

func (r *Room) getInput(inputId int) *inputChannel {

	r.inputMut.Lock()

	defer r.inputMut.Unlock()

	for _, input := range r.inputs {

		if input.id == inputId {
			return input
		}
	}
	return nil
}

func (r *Room) removeInput(id int) {
	r.inputMut.Lock()

	for idx, input := range r.inputs {

		if input.id == id {
			r.inputs[idx] = r.inputs[len(r.inputs)-1]
			r.inputs = r.inputs[:len(r.inputs)-1]
			break
		}
	}

	r.inputMut.Unlock()
}

func (r *Room) addClient(w http.ResponseWriter, token string) int {

	r.clientsMut.Lock()

	newClientid := r.currentclientId
	r.clients = append(r.clients, &roomClient{id: newClientid, token: token, channel: make(chan []int16, 1), clientConn: w})
	r.currentclientId++

	r.clientsMut.Unlock()

	return newClientid
}

func (r *Room) writeClientChannel(buf clientBuffer) error {

	r.clientsMut.Lock()

	for i := 0; i < len(r.clients); i++ {

		if r.clients[i].id == buf.clientid {
			if len(r.clients[i].channel) < 2 {
				r.clients[i].channel <- buf.buffer
			}
			break
		}
	}

	r.clientsMut.Unlock()

	return nil
}

func (r *Room) getClient(clientId int) *roomClient {

	r.clientsMut.Lock()

	defer r.clientsMut.Unlock()

	for i := 0; i < len(r.clients); i++ {

		if r.clients[i].id == clientId {
			return r.clients[i]
		}
	}

	return nil
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

func (r *Room) removeClient(id int) {
	r.clientsMut.Lock()

	for idx, client := range r.clients {

		if client.id == id {
			r.clients[idx] = r.clients[len(r.clients)-1]
			r.clients = r.clients[:len(r.clients)-1]
			break
		}
	}

	r.clientsMut.Unlock()
}

func (r *Room) isActive() bool {

	r.inputMut.Lock()
	defer r.inputMut.Unlock()

	if len(r.inputs) > 0 {
		return true
	}

	return false
}

func (r *Room) getInputsIds() []int {

	r.inputMut.Lock()
	retbuf := make([]int, len(r.inputs), len(r.inputs))
	for idx, input := range r.inputs {
		retbuf[idx] = input.id
	}
	r.inputMut.Unlock()

	return retbuf
}

func (i *inputChannel) getBuffer() *inputChannelBuffer {
	i.bufMut.Lock()
	defer i.bufMut.Unlock()

	if len(i.buffers) > 0 {
		return i.buffers[0]
	}

	return nil
}

func (i *inputChannel) readSample(curBuffer *inputChannelBuffer) (*inputChannelBuffer, int16) {

	//read sample from input
	inputSample := int16(curBuffer.buffer[curBuffer.nRead]) + (int16(curBuffer.buffer[curBuffer.nRead+1]) << 8)
	curBuffer.nRead += 2

	//unshift first buffer when all data is read
	if (curBuffer.nRead + 1) >= len(curBuffer.buffer) {

		i.bufMut.Lock()
		defer i.bufMut.Unlock()

		i.buffers = i.buffers[1:]

		//break input loop if no more buffers
		if len(i.buffers) <= 0 {
			//log.Println("starvation")
			return nil, inputSample
		}
		//update pointer to first buffer
		curBuffer = i.buffers[0]
	}

	return curBuffer, inputSample
}

func (r *Room) mixOutputChannel(time time.Time) []clientBuffer {

	nSamples := r.output.buffSize / 2

	//create output buffers for each clients
	clientBuffers := r.getClientBuffers()

	//log.Printf("out %d \n", nSamples)

	ids := r.getInputsIds()

	//iterate through room inputs
	for _, id := range ids {

		myinput := r.getInput(id)

		if myinput == nil {
			continue
		}

		curBuffer := myinput.getBuffer()
		if curBuffer == nil {
			continue
		}

		//iterate through room input samples to fill the buffer
		for nWriteChan := 0; nWriteChan < nSamples; nWriteChan++ {
			var inputSample int16

			curBuffer, inputSample = myinput.readSample(curBuffer)

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

			if curBuffer == nil {
				break
			}
		}
	}
	return clientBuffers
}
