package opusx

/*
#cgo pkg-config: opus
#include <stdlib.h>
#include <opus/opus.h>

static int go_opus_encoder_set_bitrate(OpusEncoder *enc, int bitrate) {
	return opus_encoder_ctl(enc, OPUS_SET_BITRATE(bitrate));
}
static int go_opus_encoder_set_complexity(OpusEncoder *enc, int complexity) {
	return opus_encoder_ctl(enc, OPUS_SET_COMPLEXITY(complexity));
}
static int go_opus_encoder_set_inband_fec(OpusEncoder *enc, int enable) {
	return opus_encoder_ctl(enc, OPUS_SET_INBAND_FEC(enable));
}
static int go_opus_encoder_set_packet_loss_perc(OpusEncoder *enc, int perc) {
	return opus_encoder_ctl(enc, OPUS_SET_PACKET_LOSS_PERC(perc));
}
static int go_opus_encoder_set_vbr(OpusEncoder *enc, int enable) {
	return opus_encoder_ctl(enc, OPUS_SET_VBR(enable));
}
static int go_opus_encoder_set_dred_duration(OpusEncoder *enc, int duration) {
	return opus_encoder_ctl(enc, OPUS_SET_DRED_DURATION(duration));
}
static int go_opus_encoder_set_dnn_blob(OpusEncoder *enc, void *data, int len) {
	return opus_encoder_ctl(enc, OPUS_SET_DNN_BLOB(data, len));
}

static int go_opus_decoder_set_dnn_blob(OpusDecoder *dec, void *data, int len) {
	return opus_decoder_ctl(dec, OPUS_SET_DNN_BLOB(data, len));
}
static int go_opus_dred_decoder_set_dnn_blob(OpusDREDDecoder *dred_dec, void *data, int len) {
	return opus_dred_decoder_ctl(dred_dec, OPUS_SET_DNN_BLOB(data, len));
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const (
	AppVoIP = int(C.OPUS_APPLICATION_VOIP)
)

func Version() string {
	return C.GoString(C.opus_get_version_string())
}

func opusError(code C.int) error {
	if code >= 0 {
		return nil
	}
	return fmt.Errorf("opus error(%d): %s", int(code), C.GoString(C.opus_strerror(code)))
}

type Encoder struct {
	st       *C.OpusEncoder
	channels int
}

func NewEncoder(sampleRate, channels, application int) (*Encoder, error) {
	var errCode C.int
	st := C.opus_encoder_create(
		C.opus_int32(sampleRate),
		C.int(channels),
		C.int(application),
		&errCode,
	)
	if err := opusError(errCode); err != nil {
		return nil, err
	}
	if st == nil {
		return nil, fmt.Errorf("opus_encoder_create returned nil")
	}
	return &Encoder{st: st, channels: channels}, nil
}

func (e *Encoder) Close() {
	if e == nil || e.st == nil {
		return
	}
	C.opus_encoder_destroy(e.st)
	e.st = nil
}

func (e *Encoder) SetBitrate(bitrate int) error {
	return opusError(C.go_opus_encoder_set_bitrate(e.st, C.int(bitrate)))
}

func (e *Encoder) SetComplexity(complexity int) error {
	return opusError(C.go_opus_encoder_set_complexity(e.st, C.int(complexity)))
}

func (e *Encoder) SetInBandFEC(enable bool) error {
	val := 0
	if enable {
		val = 1
	}
	return opusError(C.go_opus_encoder_set_inband_fec(e.st, C.int(val)))
}

func (e *Encoder) SetPacketLossPerc(perc int) error {
	return opusError(C.go_opus_encoder_set_packet_loss_perc(e.st, C.int(perc)))
}

func (e *Encoder) SetVBR(enable bool) error {
	val := 0
	if enable {
		val = 1
	}
	return opusError(C.go_opus_encoder_set_vbr(e.st, C.int(val)))
}

func (e *Encoder) SetDREDDuration(duration int) error {
	return opusError(C.go_opus_encoder_set_dred_duration(e.st, C.int(duration)))
}

func (e *Encoder) SetDNNBlob(blob []byte) error {
	if len(blob) == 0 {
		return fmt.Errorf("empty dnn blob")
	}
	return opusError(C.go_opus_encoder_set_dnn_blob(
		e.st,
		unsafe.Pointer(&blob[0]),
		C.int(len(blob)),
	))
}

func (e *Encoder) Encode(pcm []int16, frameSize int, packet []byte) (int, error) {
	if e == nil || e.st == nil {
		return 0, fmt.Errorf("encoder not initialized")
	}
	if len(packet) == 0 {
		return 0, fmt.Errorf("empty packet buffer")
	}
	if len(pcm) < frameSize*e.channels {
		return 0, fmt.Errorf("insufficient pcm samples: have=%d need=%d", len(pcm), frameSize*e.channels)
	}

	ret := C.opus_encode(
		e.st,
		(*C.opus_int16)(unsafe.Pointer(&pcm[0])),
		C.int(frameSize),
		(*C.uchar)(unsafe.Pointer(&packet[0])),
		C.opus_int32(len(packet)),
	)
	if err := opusError(ret); err != nil {
		return 0, err
	}
	return int(ret), nil
}

type Decoder struct {
	st         *C.OpusDecoder
	dredDec    *C.OpusDREDDecoder
	dredState  *C.OpusDRED
	channels   int
	sampleRate int
}

func NewDecoder(sampleRate, channels int) (*Decoder, error) {
	var errCode C.int
	st := C.opus_decoder_create(C.opus_int32(sampleRate), C.int(channels), &errCode)
	if err := opusError(errCode); err != nil {
		return nil, err
	}
	if st == nil {
		return nil, fmt.Errorf("opus_decoder_create returned nil")
	}
	return &Decoder{
		st:         st,
		channels:   channels,
		sampleRate: sampleRate,
	}, nil
}

func (d *Decoder) Close() {
	if d == nil {
		return
	}
	if d.dredState != nil {
		C.opus_dred_free(d.dredState)
		d.dredState = nil
	}
	if d.dredDec != nil {
		C.opus_dred_decoder_destroy(d.dredDec)
		d.dredDec = nil
	}
	if d.st != nil {
		C.opus_decoder_destroy(d.st)
		d.st = nil
	}
}

func (d *Decoder) SetDNNBlob(blob []byte) error {
	if len(blob) == 0 {
		return fmt.Errorf("empty dnn blob")
	}
	if err := opusError(C.go_opus_decoder_set_dnn_blob(
		d.st, unsafe.Pointer(&blob[0]), C.int(len(blob)),
	)); err != nil {
		return err
	}
	if d.dredDec != nil {
		if err := opusError(C.go_opus_dred_decoder_set_dnn_blob(
			d.dredDec, unsafe.Pointer(&blob[0]), C.int(len(blob)),
		)); err != nil {
			return err
		}
	}
	return nil
}

func (d *Decoder) EnableDRED(blob []byte) error {
	if len(blob) == 0 {
		return fmt.Errorf("empty dnn blob")
	}
	var errCode C.int
	d.dredDec = C.opus_dred_decoder_create(&errCode)
	if err := opusError(errCode); err != nil {
		return err
	}
	if d.dredDec == nil {
		return fmt.Errorf("opus_dred_decoder_create returned nil")
	}
	d.dredState = C.opus_dred_alloc(&errCode)
	if err := opusError(errCode); err != nil {
		return err
	}
	if d.dredState == nil {
		return fmt.Errorf("opus_dred_alloc returned nil")
	}
	return opusError(C.go_opus_dred_decoder_set_dnn_blob(
		d.dredDec,
		unsafe.Pointer(&blob[0]),
		C.int(len(blob)),
	))
}

func (d *Decoder) Decode(packet []byte, frameSize int, decodeFEC bool, pcm []int16) (int, error) {
	if len(pcm) < frameSize*d.channels {
		return 0, fmt.Errorf("insufficient pcm output buffer: have=%d need=%d", len(pcm), frameSize*d.channels)
	}
	decodeFECFlag := C.int(0)
	if decodeFEC {
		decodeFECFlag = 1
	}

	var dataPtr *C.uchar
	var dataLen C.opus_int32
	if len(packet) > 0 {
		dataPtr = (*C.uchar)(unsafe.Pointer(&packet[0]))
		dataLen = C.opus_int32(len(packet))
	}

	ret := C.opus_decode(
		d.st,
		dataPtr,
		dataLen,
		(*C.opus_int16)(unsafe.Pointer(&pcm[0])),
		C.int(frameSize),
		decodeFECFlag,
	)
	if err := opusError(ret); err != nil {
		return 0, err
	}
	return int(ret), nil
}

func (d *Decoder) DecodePLC(frameSize int, pcm []int16) (int, error) {
	return d.Decode(nil, frameSize, false, pcm)
}

func (d *Decoder) ParseDRED(packet []byte, maxDREDSamples int) (int, error) {
	if d.dredDec == nil || d.dredState == nil {
		return 0, fmt.Errorf("dred not enabled")
	}
	if len(packet) == 0 {
		return 0, nil
	}
	var dredEnd C.int
	ret := C.opus_dred_parse(
		d.dredDec,
		d.dredState,
		(*C.uchar)(unsafe.Pointer(&packet[0])),
		C.opus_int32(len(packet)),
		C.opus_int32(maxDREDSamples),
		C.opus_int32(d.sampleRate),
		&dredEnd,
		C.int(0),
	)
	if err := opusError(ret); err != nil {
		return 0, err
	}
	return int(ret), nil
}

func (d *Decoder) DecodeDRED(offsetSamples int, frameSize int, pcm []int16) (int, error) {
	if d.dredState == nil {
		return 0, fmt.Errorf("dred state not initialized")
	}
	if len(pcm) < frameSize*d.channels {
		return 0, fmt.Errorf("insufficient pcm output buffer for dred decode")
	}
	ret := C.opus_decoder_dred_decode(
		d.st,
		d.dredState,
		C.opus_int32(offsetSamples),
		(*C.opus_int16)(unsafe.Pointer(&pcm[0])),
		C.opus_int32(frameSize),
	)
	if err := opusError(ret); err != nil {
		return 0, err
	}
	return int(ret), nil
}
