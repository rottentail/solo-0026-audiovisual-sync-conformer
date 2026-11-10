package media

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/rottentail/solo-0026-audiovisual-sync-conformer/internal/domain"
)

type VideoClip struct {
	Info   domain.VideoInfo
	Frames [][]byte
}

type parsedHeader struct {
	width     int
	height    int
	fpsNum    int
	fpsDen    int
	chroma    string
	aspect    string
	interlace string
}

func InspectVideo(data []byte) (VideoClip, error) {
	lineEnd := bytes.IndexByte(data, '\n')
	if lineEnd < 0 {
		return VideoClip{}, fmt.Errorf("Y4M header line is incomplete")
	}
	header, err := parseY4MHeader(string(data[:lineEnd]))
	if err != nil {
		return VideoClip{}, err
	}
	frameBytes := header.width*header.height + 2*(header.width/2)*(header.height/2)
	if frameBytes <= 0 {
		return VideoClip{}, fmt.Errorf("Y4M frame dimensions are invalid")
	}
	frames := make([][]byte, 0, 24)
	cursor := lineEnd + 1
	for cursor < len(data) {
		headerEnd := bytes.IndexByte(data[cursor:], '\n')
		if headerEnd < 0 {
			return VideoClip{}, fmt.Errorf("Y4M frame header is incomplete")
		}
		headerEnd += cursor
		frameHeader := string(data[cursor:headerEnd])
		fields := strings.Fields(frameHeader)
		if len(fields) == 0 || fields[0] != "FRAME" {
			return VideoClip{}, fmt.Errorf("expected a Y4M FRAME marker")
		}
		bodyStart := headerEnd + 1
		bodyEnd := bodyStart + frameBytes
		if bodyEnd > len(data) {
			return VideoClip{}, fmt.Errorf("Y4M frame payload is truncated")
		}
		frame := make([]byte, frameBytes)
		copy(frame, data[bodyStart:bodyEnd])
		frames = append(frames, frame)
		if len(frames) > domain.MaxVideoFrames {
			return VideoClip{}, fmt.Errorf("Y4M frame count exceeds the limit")
		}
		cursor = bodyEnd
	}
	if len(frames) == 0 {
		return VideoClip{}, fmt.Errorf("Y4M payload has no frames")
	}
	info := domain.VideoInfo{
		Width:      header.width,
		Height:     header.height,
		FPSNum:     header.fpsNum,
		FPSDen:     header.fpsDen,
		FrameCount: len(frames),
		DurationMS: len(frames) * 1000 * header.fpsDen / header.fpsNum,
		Chroma:     header.chroma,
		Digest:     DigestBytes(data),
		ByteCount:  len(data),
	}
	return VideoClip{Info: info, Frames: frames}, nil
}

func parseY4MHeader(line string) (parsedHeader, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 || fields[0] != "YUV4MPEG2" {
		return parsedHeader{}, fmt.Errorf("payload is not Y4M")
	}
	header := parsedHeader{fpsDen: 1, chroma: "C420jpeg", aspect: "0:0", interlace: "p"}
	for _, token := range fields[1:] {
		if len(token) < 2 {
			return parsedHeader{}, fmt.Errorf("invalid Y4M header token")
		}
		value := token[1:]
		switch token[0] {
		case 'W':
			number, err := strconv.Atoi(value)
			if err != nil {
				return parsedHeader{}, fmt.Errorf("invalid Y4M width")
			}
			header.width = number
		case 'H':
			number, err := strconv.Atoi(value)
			if err != nil {
				return parsedHeader{}, fmt.Errorf("invalid Y4M height")
			}
			header.height = number
		case 'F':
			parts := strings.Split(value, ":")
			if len(parts) != 2 {
				return parsedHeader{}, fmt.Errorf("invalid Y4M frame rate")
			}
			num, errNum := strconv.Atoi(parts[0])
			den, errDen := strconv.Atoi(parts[1])
			if errNum != nil || errDen != nil || num <= 0 || den <= 0 {
				return parsedHeader{}, fmt.Errorf("invalid Y4M frame rate")
			}
			header.fpsNum = num
			header.fpsDen = den
		case 'C':
			header.chroma = token
		case 'A':
			header.aspect = value
		case 'I':
			header.interlace = value
		}
	}
	if header.width <= 0 || header.height <= 0 {
		return parsedHeader{}, fmt.Errorf("Y4M width and height are required")
	}
	if header.width%2 != 0 || header.height%2 != 0 {
		return parsedHeader{}, fmt.Errorf("C420 dimensions must be even")
	}
	if header.width > domain.MaxVideoWidth || header.height > domain.MaxVideoHeight {
		return parsedHeader{}, fmt.Errorf("Y4M dimensions exceed the limit")
	}
	if header.fpsNum <= 0 || header.fpsNum > 120 || header.fpsDen > 1001 {
		return parsedHeader{}, fmt.Errorf("Y4M frame rate is outside the accepted range")
	}
	if header.chroma != "C420jpeg" && header.chroma != "C420mpeg2" && header.chroma != "C420" {
		return parsedHeader{}, fmt.Errorf("only planar C420 Y4M is accepted")
	}
	if header.interlace != "p" {
		return parsedHeader{}, fmt.Errorf("only progressive Y4M is accepted")
	}
	return header, nil
}

func EncodeY4M(info domain.VideoInfo, frames [][]byte) ([]byte, error) {
	if info.Width <= 0 || info.Height <= 0 || info.FPSNum <= 0 || info.FPSDen <= 0 {
		return nil, fmt.Errorf("invalid Y4M output geometry")
	}
	chroma := info.Chroma
	if chroma == "" {
		chroma = "C420jpeg"
	}
	frameBytes := info.Width*info.Height + 2*(info.Width/2)*(info.Height/2)
	var output bytes.Buffer
	fmt.Fprintf(
		&output,
		"YUV4MPEG2 W%d H%d F%d:%d Ip A0:0 %s\n",
		info.Width,
		info.Height,
		info.FPSNum,
		info.FPSDen,
		chroma,
	)
	for _, frame := range frames {
		if len(frame) != frameBytes {
			return nil, fmt.Errorf("Y4M frame byte count does not match geometry")
		}
		output.WriteString("FRAME\n")
		output.Write(frame)
	}
	return output.Bytes(), nil
}

func NormalizeY4M(data []byte) ([]byte, domain.VideoInfo, error) {
	clip, err := InspectVideo(data)
	if err != nil {
		return nil, domain.VideoInfo{}, err
	}
	output, err := EncodeY4M(clip.Info, clip.Frames)
	if err != nil {
		return nil, domain.VideoInfo{}, err
	}
	info := clip.Info
	info.Digest = DigestBytes(output)
	info.ByteCount = len(output)
	return output, info, nil
}
