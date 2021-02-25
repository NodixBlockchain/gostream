package main

import "C"

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
	"github.com/xlab/vorbis-go/vorbis"
	"gopkg.in/hraban/opus.v2"
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

type WavEncoder struct {
	w            http.ResponseWriter
	SampleRate   int
	BitDepth     int
	NumChans     int
	WrittenBytes int
}

func NewWavEncoder(w http.ResponseWriter, outChan outputChannel) *WavEncoder {
	return &WavEncoder{
		w:            w,
		SampleRate:   outChan.sampleRate,
		BitDepth:     16,
		NumChans:     outChan.channels,
		WrittenBytes: 0}
}

func (e *WavEncoder) AddLE(src interface{}) error {
	e.WrittenBytes += binary.Size(src)
	return binary.Write(e.w, binary.LittleEndian, src)
}

func (e *WavEncoder) writeHeader() error {

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

type OggVorbisEncoder struct {
	w          http.ResponseWriter
	SampleRate int
	BitDepth   int
	NumChans   int

	vi vorbis.Info
	vd vorbis.DspState
	vb vorbis.Block
	os vorbis.OggStreamState
	vc vorbis.Comment
}

func NewOggEncoder(w http.ResponseWriter, outChan outputChannel) *OggVorbisEncoder {

	return &OggVorbisEncoder{
		w:          w,
		SampleRate: outChan.sampleRate,
		BitDepth:   16,
		NumChans:   outChan.channels}
}

func (e *OggVorbisEncoder) writeHeader() int32 {

	var header, header_comm, header_code vorbis.OggPacket
	var og vorbis.OggPage

	vorbis.InfoInit(&e.vi)
	vorbis.CommentInit(&e.vc)

	ve := vorbis.EncodeInitVbr(&e.vi, e.NumChans, e.SampleRate, 0.4)

	if ve != 0 {
		log.Println("error new ogg encoder ", ve)
		return ve
	}

	ve = vorbis.AnalysisInit(&e.vd, &e.vi)

	ve = vorbis.BlockInit(&e.vd, &e.vb)

	ve = vorbis.OggStreamInit(&e.os, 1)

	ve = vorbis.AnalysisHeaderout(&e.vd, &e.vc, &header, &header_comm, &header_code)

	ve = vorbis.OggStreamPacketin(&e.os, &header)
	ve = vorbis.OggStreamPacketin(&e.os, &header_comm)
	ve = vorbis.OggStreamPacketin(&e.os, &header_code)

	for vorbis.OggStreamFlush(&e.os, &og) != 0 {
		e.w.Write(og.Header)
		e.w.Write(og.Body)
	}

	return ve
}

func (e *OggVorbisEncoder) writeBuffer(newBuffer []int16) int32 {

	var og vorbis.OggPage
	var op vorbis.OggPacket

	bufLen := len(newBuffer)

	goBuff := make([]float32, bufLen, bufLen)

	for i := 0; i < bufLen; i++ {
		goBuff[i] = float32(newBuffer[i]) / 32768.0
	}

	ve := vorbis.AnalysisWriteBuffer(&e.vd, goBuff, int32(bufLen))

	if ve != 0 {
		fmt.Println("vorbis.AnalysisWrote failed:", ve)
		return ve
	}

	/* vorbis does some data preanalysis, then divvies up blocks for
	more involved (potentially parallel) processing.  Get a single
	block for encoding now */
	for vorbis.AnalysisBlockout(&e.vd, &e.vb) == 1 {

		/* analysis, assume we want to use bitrate management */
		ve = vorbis.Analysis(&e.vb, nil)

		if ve != 0 {
			fmt.Println("vorbis.Analysis failed:", ve)
			return ve
		}

		ve = vorbis.BitrateAddblock(&e.vb)

		if ve != 0 {
			fmt.Println("vorbis.BitrateAddblock failed:", ve)
			return ve
		}

		for vorbis.BitrateFlushpacket(&e.vd, &op) != 0 {

			/* weld the packet into the bitstream */
			ve = vorbis.OggStreamPacketin(&e.os, &op)

			if ve != 0 {
				fmt.Println("vorbis.OggStreamPacketin failed:", ve)
				return ve
			}

			/* write out pages (if any) */
			for vorbis.OggStreamPageout(&e.os, &og) != 0 {

				e.w.Write(og.Header)
				e.w.Write(og.Body)
			}
		}
	}

	return ve
}

type opusOGGHeader struct {
	version int // The Ogg Opus format version, in the range 0...255. More...

	channel_count int //The number of channels, in the range 1...255. More...

	pre_skip uint //The number of samples that should be discarded from the beginning of the stream. More...

	input_sample_rate uint32 //The sampling rate of the original input. More...
	output_gain       int    //The gain to apply to the decoded output, in dB, as a Q8 value in the range -32768...32767. More...

	mapping_family int //The channel mapping family, in the range 0...255. More...

	stream_count int //The number of Opus streams in each Ogg packet, in the range 1...255. More...

	coupled_count int //The number of coupled Opus streams in each Ogg packet, in the range 0...127. More...

	mapping [255]byte
}

type OggOpusEncoder struct {
	w           http.ResponseWriter
	SampleRate  int
	NumChans    int
	frameSize   int
	frameSizeMs float32

	encdata      []byte
	opusEnc      *opus.Encoder
	samplesWrote int64

	useOgg   bool
	Packetno vorbis.OggInt64
	opusHDR  opusOGGHeader
	os       vorbis.OggStreamState
	vc       vorbis.Comment
}

func NewOpusEncoder(w http.ResponseWriter, useOgg bool, outChan outputChannel) *OggOpusEncoder {

	return &OggOpusEncoder{
		w:            w,
		samplesWrote: 0,
		useOgg:       useOgg,
		Packetno:     0,
		SampleRate:   outChan.sampleRate,
		NumChans:     outChan.channels}
}

func (e *OggOpusEncoder) PutUint32(b []byte, v uint32) []byte {
	var r []byte

	r = append(b, byte(v))
	r = append(r, byte(v>>8))
	r = append(r, byte(v>>16))
	r = append(r, byte(v>>24))

	return r
}

func (e *OggOpusEncoder) PutUint16(b []byte, v uint16) []byte {
	var r []byte

	r = append(b, byte(v))
	r = append(r, byte(v>>8))

	return r
}

func (e *OggOpusEncoder) PutInt16(b []byte, v int16) []byte {
	var r []byte
	r = append(b, byte(v))
	r = append(r, byte(v>>8))
	return r
}

func (e *OggOpusEncoder) writeHeader() error {

	var err error

	e.frameSize = (10 * e.SampleRate) / 1000
	e.frameSizeMs = float32(e.frameSize) * 1000.0 / float32(e.SampleRate)

	switch e.frameSizeMs {
	case 2.5, 5, 10, 20, 40, 60:
		// Good.
	default:
		return fmt.Errorf("Illegal frame size")
	}

	if e.useOgg {
		var og vorbis.OggPage
		var header, comments vorbis.OggPacket
		var packetBuffer []byte

		e.opusHDR.version = 1
		e.opusHDR.channel_count = e.NumChans
		e.opusHDR.pre_skip = 0
		e.opusHDR.input_sample_rate = uint32(e.SampleRate)
		e.opusHDR.output_gain = 0
		e.opusHDR.mapping_family = 0
		e.opusHDR.stream_count = 1
		e.opusHDR.coupled_count = 0
		e.opusHDR.mapping[0] = 0

		packetBuffer = []byte("OpusHead")
		packetBuffer = append(packetBuffer, byte(e.opusHDR.version))
		packetBuffer = append(packetBuffer, byte(e.opusHDR.channel_count))
		packetBuffer = e.PutUint16(packetBuffer, uint16(e.opusHDR.pre_skip))
		packetBuffer = e.PutUint32(packetBuffer, uint32(e.opusHDR.input_sample_rate))
		packetBuffer = e.PutInt16(packetBuffer, int16(e.opusHDR.output_gain))
		packetBuffer = append(packetBuffer, byte(e.opusHDR.mapping_family))

		if e.opusHDR.mapping_family != 0 {
			packetBuffer = append(packetBuffer, byte(e.opusHDR.stream_count))
			for i := 0; i < e.NumChans; i++ {
				packetBuffer = append(packetBuffer, e.opusHDR.mapping[i])
			}

		}
		//packetBuffer = append(packetBuffer, e.opusHDR.mapping[:]...)

		//log.Printf("packetBuffer : %x", packetBuffer)

		header.Packet = packetBuffer
		header.Bytes = len(packetBuffer)
		header.BOS = 1
		header.EOS = 0
		header.Granulepos = 0
		header.Packetno = 0

		ve := vorbis.OggStreamInit(&e.os, 1)

		if ve != 0 {
			return fmt.Errorf("unable to initialize ogg stream ")
		}
		ve = vorbis.MyOggStreamPacketin(&e.os, &header)

		if ve != 0 {
			return fmt.Errorf("unable to write header packet")
		}

		for vorbis.OggStreamFlush(&e.os, &og) != 0 {
			e.w.Write(og.Header)
			e.w.Write(og.Body)
		}

		/*
			vorbis.CommentInit(&e.vc)
			vorbis.CommentAdd(&e.vc, "ARTIST=hello")
			vorbis.CommentAdd(&e.vc, "ENCODER=gostream")
			vorbis.CommentAdd(&e.vc, "TITLE=room")
			vorbis.CommentheaderOut(&e.vc, &comments)
		*/

		encoderName := "goStream"

		packetBuffer = []byte("OpusTags")
		packetBuffer = e.PutUint32(packetBuffer, uint32(len(encoderName)))
		packetBuffer = append(packetBuffer, []byte(encoderName)...)
		packetBuffer = e.PutUint32(packetBuffer, uint32(0))

		comments.Packet = packetBuffer
		comments.Bytes = len(packetBuffer)
		comments.BOS = 0
		comments.EOS = 0
		comments.Granulepos = 0
		comments.Packetno = 1

		ve = vorbis.MyOggStreamPacketin(&e.os, &comments)

		if ve != 0 {
			return fmt.Errorf("unable to write comment packet")
		}

		for vorbis.OggStreamFlush(&e.os, &og) != 0 {
			e.w.Write(og.Header[:og.HeaderLen])
			e.w.Write(og.Body[:og.BodyLen])
		}
		e.Packetno = 2
	}

	e.opusEnc, err = opus.NewEncoder(e.SampleRate, e.NumChans, opus.AppVoIP)
	if err != nil {
		return fmt.Errorf("error initializing  opus encoder")
	}
	e.encdata = make([]byte, 1000)

	return nil
}

func (e *OggOpusEncoder) writeBuffer(newBuffer []int16) error {

	n, err := e.opusEnc.Encode(newBuffer, e.encdata)
	if err != nil {
		return fmt.Errorf("error encoding")
	}

	if e.useOgg {

		var og vorbis.OggPage
		var op vorbis.OggPacket

		op.Packet = e.encdata[:n]
		op.Bytes = n
		op.BOS = 0
		op.EOS = 0
		op.Granulepos = vorbis.OggInt64(e.samplesWrote)
		op.Packetno = e.Packetno

		log.Println("ogg gp ", op.Granulepos)

		ve := vorbis.MyOggStreamPacketin(&e.os, &op)
		if ve != 0 {
			return fmt.Errorf("error packetizing opus")
		}

		for vorbis.OggStreamPageout(&e.os, &og) != 0 {
			e.w.Write(og.Header[:og.HeaderLen])
			e.w.Write(og.Body[:og.BodyLen])
		}
	} else {
		e.w.Write(e.encdata[:n])
	}

	e.samplesWrote += int64(len(newBuffer))
	e.Packetno++

	/*
		for nFrame := 0; nFrame < len(newBuffer); nFrame += frameSize {

			log.Println(" buffer : ", len(newBuffer[nFrame:nFrame+frameSize]))

			n, err := e.Encode(newBuffer[nFrame:nFrame+frameSize], data)
			if err != nil {
				http.Error(w, "error encoding opus", http.StatusInternalServerError)
				return
			}

			totalWrote += vorbis.OggInt64(frameSize)

			op.Packet = data[:n]
			op.Bytes = n
			op.BOS = 0
			op.EOS = 0
			op.Granulepos = totalWrote
			op.Packetno = Packetno
			ve = vorbis.MyOggStreamPacketin(&os, &op)
			if ve != 0 {
				http.Error(w, "error encoding ogg", http.StatusInternalServerError)
				return
			}

			Packetno++

			// only the first N bytes are opus data. Just like io.Reader.
			//w.Write(data[:n])
		}
	*/

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

	w.Header().Set("content-type", "appplication/stream")
	//w.Header().Set("Transfer-Encoding", "chunked")
	//w.Header().Set("content-Length", "2400000")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(200)

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

	/*
		e := NewEncoderWav(roomList[roomFound].clients[newClientIndex].clientConn, roomList[roomFound].output)
		err = e.writeHeader()
		if err != nil {
			http.Error(w, "error writing wav header  ", http.StatusInternalServerError)
			return
		}
	*/

	/*
		e := NewOggEncoder(roomList[roomFound].clients[newClientIndex].clientConn, roomList[roomFound].output)
		oggerr := e.writeHeader()
		if oggerr != 0 {
			http.Error(w, "error writing ogg header  ", http.StatusInternalServerError)
			return
		}
	*/

	e := NewOpusEncoder(roomList[roomFound].clients[newClientIndex].clientConn, true, roomList[roomFound].output)
	opuserr := e.writeHeader()
	if opuserr != nil {
		http.Error(w, "error writing opus header  ", http.StatusInternalServerError)
		return
	}

	for {
		newBuffer := <-roomList[roomFound].clients[newClientIndex].channel
		e.writeBuffer(newBuffer)
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
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
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

	http.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("./js"))))
	http.Handle("/html/", http.StripPrefix("/html/", http.FileServer(http.Dir("./html"))))
	http.HandleFunc("/joinRoom", handleJoinRoom)
	log.Fatal(http.ListenAndServe(":8081", nil))

}
