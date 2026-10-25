package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/model"
)

// buildVideoRenderFFmpegArgs constructs a deterministic ffmpeg argv for one timeline.
// Image shots are held as still frames; video shots are trimmed; optional BGM mixes under
// a silent base track; burn-in subtitles use drawtext (no ASS dependency).
func buildVideoRenderFFmpegArgs(workDir string, timeline model.VideoTimeline, shotFiles map[string]string, bgmFile, outputPath string) ([]string, error) {
	if len(timeline.Shots) == 0 {
		return nil, fmt.Errorf("timeline has no shots")
	}
	out := timeline.Output
	if out.Width <= 0 || out.Height <= 0 || out.FPS <= 0 {
		return nil, fmt.Errorf("invalid output spec")
	}
	args := []string{"-y", "-hide_banner", "-loglevel", "error"}
	filterParts := []string{}
	concatInputs := []string{}
	inputIndex := 0
	for _, shot := range timeline.Shots {
		src := shotFiles[shot.ID]
		if src == "" {
			return nil, fmt.Errorf("missing materialized file for shot %s", shot.ID)
		}
		durationSec := float64(shot.DurationMs) / 1000.0
		if shot.Source.Kind == "image" {
			args = append(args, "-loop", "1", "-t", fmt.Sprintf("%.3f", durationSec), "-i", src)
		} else {
			trimStart := float64(shot.TrimStartMs) / 1000.0
			args = append(args, "-ss", fmt.Sprintf("%.3f", trimStart), "-t", fmt.Sprintf("%.3f", durationSec), "-i", src)
		}
		scale := fmt.Sprintf("[%d:v]scale=%d:%d:force_original_aspect_ratio=", inputIndex, out.Width, out.Height)
		if shot.CropMode == "contain" {
			scale += "decrease,pad=" + strconv.Itoa(out.Width) + ":" + strconv.Itoa(out.Height) + ":(ow-iw)/2:(oh-ih)/2:black"
		} else {
			scale += "increase,crop=" + strconv.Itoa(out.Width) + ":" + strconv.Itoa(out.Height)
		}
		label := fmt.Sprintf("v%d", inputIndex)
		filterParts = append(filterParts, scale+fmt.Sprintf(",setsar=1,fps=%d,format=yuv420p[%s]", out.FPS, label))
		concatInputs = append(concatInputs, "["+label+"]")
		inputIndex++
	}
	filterParts = append(filterParts, strings.Join(concatInputs, "")+fmt.Sprintf("concat=n=%d:v=1:a=0[vout]", len(concatInputs)))
	videoLabel := "[vout]"
	for i, subtitle := range timeline.Subtitles {
		escaped := ffmpegDrawtextEscape(subtitle.Text)
		yExpr := fmt.Sprintf("h*%d/100", subtitle.PositionY)
		styleColor := "white"
		if subtitle.Style == "dark" {
			styleColor = "black"
		} else if subtitle.Style == "light" {
			styleColor = "white"
		}
		outLabel := fmt.Sprintf("[vsub%d]", i)
		filterParts = append(filterParts, fmt.Sprintf("%sdrawtext=text='%s':fontsize=36:fontcolor=%s:borderw=2:bordercolor=black:x=(w-text_w)/2:y=%s:enable='between(t,%.3f,%.3f)'%s",
			videoLabel, escaped, styleColor, yExpr, float64(subtitle.StartMs)/1000.0, float64(subtitle.EndMs)/1000.0, outLabel))
		videoLabel = outLabel
	}
	args = append(args, "-f", "lavfi", "-t", fmt.Sprintf("%.3f", float64(timelineDurationMs(timeline))/1000.0), "-i", "anullsrc=channel_layout=stereo:sample_rate=44100")
	audioIndex := inputIndex
	inputIndex++
	audioLabel := fmt.Sprintf("[%d:a]", audioIndex)
	if timeline.BGM != nil && bgmFile != "" {
		args = append(args, "-i", bgmFile)
		bgmIndex := inputIndex
		inputIndex++
		vol := float64(timeline.BGM.Volume) / 100.0
		if vol <= 0 {
			vol = 0.01
		}
		filterParts = append(filterParts, fmt.Sprintf("[%d:a]volume=%.3f,afade=t=in:st=0:d=%.3f,afade=t=out:st=%.3f:d=%.3f[bgm]",
			bgmIndex, vol,
			float64(timeline.BGM.FadeInMs)/1000.0,
			float64(timelineDurationMs(timeline)-timeline.BGM.FadeOutMs)/1000.0,
			float64(timeline.BGM.FadeOutMs)/1000.0,
		))
		filterParts = append(filterParts, fmt.Sprintf("%s[bgm]amix=inputs=2:duration=first:dropout_transition=0[aout]", audioLabel))
		audioLabel = "[aout]"
	} else {
		filterParts = append(filterParts, audioLabel+"anull[aout]")
		audioLabel = "[aout]"
	}
	args = append(args, "-filter_complex", strings.Join(filterParts, ";"), "-map", videoLabel, "-map", audioLabel)
	args = append(args,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
		"-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart",
		"-t", fmt.Sprintf("%.3f", float64(timelineDurationMs(timeline))/1000.0),
		outputPath,
	)
	_ = workDir
	return args, nil
}

func timelineDurationMs(timeline model.VideoTimeline) int {
	duration := 0
	for _, shot := range timeline.Shots {
		if end := shot.StartMs + shot.DurationMs; end > duration {
			duration = end
		}
	}
	return duration
}

func ffmpegDrawtextEscape(text string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "'", "\\'", ":", "\\:", "%", "%%")
	return replacer.Replace(text)
}

func ffmpegBinary() string {
	if strings.TrimSpace(config.Cfg.FFmpegPath) != "" {
		return strings.TrimSpace(config.Cfg.FFmpegPath)
	}
	return "ffmpeg"
}

func runVideoRenderFFmpeg(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, ffmpegBinary(), args...)
	cmd.Stdout = nil
	stderr, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(stderr))
		if len(msg) > 400 {
			msg = msg[:400]
		}
		return fmt.Errorf("ffmpeg failed: %w (%s)", err, msg)
	}
	return nil
}

func ensureWorkDir(prefix string) (string, error) {
	return os.MkdirTemp("", prefix)
}

func materializeShotPath(workDir, shotID, ext string) string {
	return filepath.Join(workDir, "shot-"+shotID+ext)
}
