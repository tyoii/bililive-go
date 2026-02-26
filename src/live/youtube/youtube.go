package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/live"
	"github.com/bililive-go/bililive-go/src/live/internal"
)

const (
	domain    = "www.youtube.com"
	domainAlt = "youtube.com"
	cnName    = "YouTube"
)

func init() {
	live.Register(domain, new(builder))
	live.Register(domainAlt, new(builder))
}

type builder struct{}

func (b *builder) Build(url *url.URL) (live.Live, error) {
	return &Live{
		BaseLive: internal.NewBaseLive(url),
	}, nil
}

type Live struct {
	internal.BaseLive
	hostName string
	roomName string
}

func (l *Live) getVideoURL() string {
	return l.Url.String()
}

func (l *Live) GetInfo() (info *live.Info, err error) {
	videoURL := l.getVideoURL()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp",
		"--no-download",
		"--print", `{"title":"%(title)s","uploader":"%(uploader)s","is_live":%(is_live)s}`,
		"--no-warnings",
		"--quiet",
		videoURL,
	)
	output, err := cmd.Output()
	if err != nil {
		if l.hostName == "" {
			l.hostName = l.parseChannelName()
		}
		if l.roomName == "" {
			l.roomName = "offline"
		}
		return &live.Info{
			Live:     l,
			HostName: l.hostName,
			RoomName: l.roomName,
			Status:   false,
		}, nil
	}

	outputStr := strings.TrimSpace(string(output))
	// yt-dlp uses Python-style booleans
	outputStr = strings.ReplaceAll(outputStr, ":True", ":true")
	outputStr = strings.ReplaceAll(outputStr, ":False", ":false")
	outputStr = strings.ReplaceAll(outputStr, ":None", ":null")

	var parsed struct {
		Title    string `json:"title"`
		Uploader string `json:"uploader"`
		IsLive   bool   `json:"is_live"`
	}
	if jsonErr := json.Unmarshal([]byte(outputStr), &parsed); jsonErr != nil {
		return &live.Info{
			Live:     l,
			HostName: l.parseChannelName(),
			RoomName: "parse error",
			Status:   false,
		}, nil
	}

	l.hostName = parsed.Uploader
	l.roomName = parsed.Title

	return &live.Info{
		Live:     l,
		HostName: l.hostName,
		RoomName: l.roomName,
		Status:   parsed.IsLive,
	}, nil
}

func (l *Live) GetStreamUrls() ([]*url.URL, error) {
	videoURL := l.getVideoURL()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp",
		"--get-url",
		"-f", "best",
		"--no-warnings",
		"--quiet",
		videoURL,
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp failed: %w", err)
	}

	streamURL := strings.TrimSpace(string(output))
	if streamURL == "" {
		return nil, fmt.Errorf("no stream URL found")
	}

	lines := strings.Split(streamURL, "\n")
	u, err := url.Parse(lines[0])
	if err != nil {
		return nil, fmt.Errorf("invalid stream URL: %w", err)
	}

	return []*url.URL{u}, nil
}

func (l *Live) GetPlatformCNName() string {
	return cnName
}

func (l *Live) parseChannelName() string {
	path := l.Url.Path
	if strings.HasPrefix(path, "/@") {
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			return strings.TrimPrefix(parts[1], "@")
		}
	}
	if strings.HasPrefix(path, "/channel/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			return parts[2]
		}
	}
	if v := l.Url.Query().Get("v"); v != "" {
		return v
	}
	return "unknown"
}
