package wav

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type PCM16Mono struct {
	SampleRate int
	Samples    []int16
}

func ReadPCM16Mono(path string) (PCM16Mono, error) {
	f, err := os.Open(path)
	if err != nil {
		return PCM16Mono{}, fmt.Errorf("open wav failed: %w", err)
	}
	defer f.Close()

	fileHeader := make([]byte, 12)
	if _, err := io.ReadFull(f, fileHeader); err != nil {
		return PCM16Mono{}, fmt.Errorf("read wav file header failed: %w", err)
	}
	if string(fileHeader[0:4]) != "RIFF" || string(fileHeader[8:12]) != "WAVE" {
		return PCM16Mono{}, fmt.Errorf("invalid wav magic")
	}

	var (
		audioFormat   uint16
		numChannels   uint16
		sampleRate    uint32
		bitsPerSample uint16
		dataBytes     []byte
		gotFmt        bool
		gotData       bool
	)

	for {
		chunkHeader := make([]byte, 8)
		_, err := io.ReadFull(f, chunkHeader)
		if err == io.EOF {
			break
		}
		if err != nil {
			return PCM16Mono{}, fmt.Errorf("read wav chunk header failed: %w", err)
		}

		chunkID := string(chunkHeader[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[4:8])
		if chunkSize > 512*1024*1024 {
			return PCM16Mono{}, fmt.Errorf("wav chunk too large: %d", chunkSize)
		}

		chunkData := make([]byte, chunkSize)
		if _, err := io.ReadFull(f, chunkData); err != nil {
			return PCM16Mono{}, fmt.Errorf("read wav chunk data(%s) failed: %w", chunkID, err)
		}
		// RIFF chunks are word-aligned.
		if chunkSize%2 == 1 {
			if _, err := f.Seek(1, io.SeekCurrent); err != nil {
				return PCM16Mono{}, fmt.Errorf("seek wav pad byte failed: %w", err)
			}
		}

		switch chunkID {
		case "fmt ":
			if len(chunkData) < 16 {
				return PCM16Mono{}, fmt.Errorf("invalid fmt chunk size=%d", len(chunkData))
			}
			audioFormat = binary.LittleEndian.Uint16(chunkData[0:2])
			numChannels = binary.LittleEndian.Uint16(chunkData[2:4])
			sampleRate = binary.LittleEndian.Uint32(chunkData[4:8])
			bitsPerSample = binary.LittleEndian.Uint16(chunkData[14:16])
			gotFmt = true
		case "data":
			dataBytes = chunkData
			gotData = true
		}
		if gotFmt && gotData {
			break
		}
	}

	if !gotFmt || !gotData {
		return PCM16Mono{}, fmt.Errorf("wav missing required fmt/data chunks")
	}

	if audioFormat != 1 {
		return PCM16Mono{}, fmt.Errorf("unsupported wav format=%d, only PCM is supported", audioFormat)
	}
	if numChannels != 1 {
		return PCM16Mono{}, fmt.Errorf("unsupported channels=%d, only mono is supported", numChannels)
	}
	if bitsPerSample != 16 {
		return PCM16Mono{}, fmt.Errorf("unsupported bits_per_sample=%d, only 16-bit is supported", bitsPerSample)
	}
	if len(dataBytes)%2 != 0 {
		return PCM16Mono{}, fmt.Errorf("invalid data chunk size=%d", len(dataBytes))
	}

	samples := make([]int16, len(dataBytes)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(dataBytes[i*2 : i*2+2]))
	}
	return PCM16Mono{
		SampleRate: int(sampleRate),
		Samples:    samples,
	}, nil
}

func WritePCM16Mono(path string, sampleRate int, samples []int16) error {
	if sampleRate <= 0 {
		return fmt.Errorf("invalid sample rate: %d", sampleRate)
	}

	dataSize := len(samples) * 2
	fileSizeMinus8 := 36 + dataSize
	byteRate := sampleRate * 2
	blockAlign := 2

	header := make([]byte, 44)
	copy(header[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(header[4:8], uint32(fileSizeMinus8))
	copy(header[8:12], []byte("WAVE"))
	copy(header[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], 1)
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(header[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize))

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create wav failed: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(header); err != nil {
		return fmt.Errorf("write wav header failed: %w", err)
	}

	raw := make([]byte, dataSize)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(raw[i*2:i*2+2], uint16(sample))
	}
	if _, err := f.Write(raw); err != nil {
		return fmt.Errorf("write wav data failed: %w", err)
	}
	return nil
}
