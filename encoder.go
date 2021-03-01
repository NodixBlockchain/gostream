package main

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"strings"

	"github.com/NodixBlockchain/vorbis-go/vorbis"
	"github.com/go-audio/riff"
	"gopkg.in/hraban/opus.v2"
)

type AudioEncoder interface {
	writeHeader() error
	writeBuffer(newBuffer []int16) error
}

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

func (e *WavEncoder) writeBuffer(newBuffer []int16) error {
	return binary.Write(e.w, binary.LittleEndian, newBuffer)
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

func (e *OggVorbisEncoder) writeHeader() error {

	var header, header_comm, header_code vorbis.OggPacket
	var og vorbis.OggPage

	vorbis.InfoInit(&e.vi)
	vorbis.CommentInit(&e.vc)

	ve := vorbis.EncodeInitVbr(&e.vi, e.NumChans, e.SampleRate, 0.4)

	if ve != 0 {
		return fmt.Errorf("error new ogg encoder %d ", ve)
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

	return nil
}

func (e *OggVorbisEncoder) writeBuffer(newBuffer []int16) error {

	var og vorbis.OggPage
	var op vorbis.OggPacket

	bufLen := len(newBuffer)

	goBuff := make([]float32, bufLen, bufLen)

	for i := 0; i < bufLen; i++ {
		goBuff[i] = float32(newBuffer[i]) / 32768.0
	}

	ve := vorbis.AnalysisWriteBuffer(&e.vd, goBuff, int32(bufLen))

	if ve != 0 {
		return fmt.Errorf("vorbis.AnalysisWrote failed: %d", ve)
	}

	/* vorbis does some data preanalysis, then divvies up blocks for
	more involved (potentially parallel) processing.  Get a single
	block for encoding now */
	for vorbis.AnalysisBlockout(&e.vd, &e.vb) == 1 {

		/* analysis, assume we want to use bitrate management */
		ve = vorbis.Analysis(&e.vb, nil)

		if ve != 0 {
			return fmt.Errorf("vorbis.Analysis failed: %d", ve)
		}

		ve = vorbis.BitrateAddblock(&e.vb)

		if ve != 0 {
			return fmt.Errorf("vorbis.BitrateAddblock failed: %d", ve)
		}

		for vorbis.BitrateFlushpacket(&e.vd, &op) != 0 {

			/* weld the packet into the bitstream */
			ve = vorbis.OggStreamPacketin(&e.os, &op)

			if ve != 0 {
				return fmt.Errorf("vorbis.OggStreamPacketin failed: %d", ve)
			}

			/* write out pages (if any) */
			for vorbis.OggStreamPageout(&e.os, &og) != 0 {

				e.w.Write(og.Header)
				e.w.Write(og.Body)
			}
		}
	}

	return nil
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
		//var packetBuffer []byte

		e.opusEnc, err = opus.NewEncoder(e.SampleRate, e.NumChans, opus.AppVoIP)
		if err != nil {
			return fmt.Errorf("error initializing  opus encoder")
		}

		/*
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
		*/

		ve := vorbis.OpusHeaderout("goStream", e.SampleRate, e.NumChans, &header, &comments)
		if ve != 0 {
			return fmt.Errorf("unable to initialize ogg headers error %d", ve)
		}

		ve = vorbis.OggStreamInit(&e.os, 1)

		if ve != 0 {
			return fmt.Errorf("unable to initialize ogg stream error %d", ve)
		}
		ve = vorbis.MyOggStreamPacketin(&e.os, &header)

		if ve != 0 {
			return fmt.Errorf("unable to write header packet error %d", ve)
		}

		for vorbis.OggStreamFlush(&e.os, &og) != 0 {
			e.w.Write(og.Header)
			e.w.Write(og.Body)
		}

		/*
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
		*/

		ve = vorbis.MyOggStreamPacketin(&e.os, &comments)

		if ve != 0 {
			return fmt.Errorf("unable to write comment packet error %d", ve)
		}

		for vorbis.OggStreamFlush(&e.os, &og) != 0 {
			e.w.Write(og.Header[:og.HeaderLen])
			e.w.Write(og.Body[:og.BodyLen])
		}
		e.Packetno = 2
	}

	e.encdata = make([]byte, 1000)

	return nil
}

func (e *OggOpusEncoder) writeBuffer(newBuffer []int16) error {

	n, err := e.opusEnc.Encode(newBuffer, e.encdata)
	if err != nil {
		return fmt.Errorf("error encoding error %v ", err)
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

		ve := vorbis.MyOggStreamPacketin(&e.os, &op)
		if ve != 0 {
			return fmt.Errorf("error packetizing opus error %d", ve)
		}

		for vorbis.OggStreamPageout(&e.os, &og) != 0 {
			nWrote, err := e.w.Write(og.Header[:og.HeaderLen])

			if (err != nil) || (nWrote < og.HeaderLen) {
				return fmt.Errorf("write opus page io error %d/%d %v", nWrote, og.HeaderLen, err)
			}

			nWrote, err = e.w.Write(og.Body[:og.BodyLen])

			if (err != nil) || (nWrote < og.BodyLen) {
				return fmt.Errorf("write opus page io error  %d/%d %v", nWrote, og.BodyLen, err)
			}

		}
	} else {
		nWrote, err := e.w.Write(e.encdata[:n])

		if (err != nil) || (nWrote < n) {
			return fmt.Errorf("write opus packget io error %d/%d %v", nWrote, n, err)
		}
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

func getEncoder(format string, w http.ResponseWriter, outChan outputChannel) AudioEncoder {

	if strings.EqualFold(format, "vorbis") {
		return NewOggEncoder(w, outChan)
	}

	if strings.EqualFold(format, "opus") {
		return NewOpusEncoder(w, true, outChan)
	}

	return NewWavEncoder(w, outChan)

}
