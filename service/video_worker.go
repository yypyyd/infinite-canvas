package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func RunVideoRenderWorker(ctx context.Context, concurrency, tenantConcurrency int) error {
	if concurrency < 1 {
		concurrency = 1
	}
	if tenantConcurrency < 1 {
		tenantConcurrency = 1
	}
	logger := slog.Default()
	logger.Info("worker_event", "component", "worker", "worker", "video_render", "event", "started", "concurrency", concurrency, "tenant_concurrency", tenantConcurrency)
	sem := make(chan struct{}, concurrency)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		job, claimed, err := ClaimVideoRenderJob(tenantConcurrency)
		if err != nil {
			logger.Error("worker_event", "component", "worker", "worker", "video_render", "event", "claim_failed", "error_type", fmt.Sprintf("%T", err))
			waitVideoWorker(ctx, 2*time.Second)
			continue
		}
		if !claimed {
			waitVideoWorker(ctx, 2*time.Second)
			continue
		}
		sem <- struct{}{}
		go func(job model.VideoRenderJob) {
			defer func() { <-sem }()
			executeVideoRenderJob(ctx, job)
		}(job)
	}
}

func waitVideoWorker(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func executeVideoRenderJob(parent context.Context, job model.VideoRenderJob) {
	logger := slog.Default()
	attrs := []any{"component", "worker", "worker", "video_render", "job_id", job.ID, "organization_id", job.OrganizationID, "attempt", job.Attempt}
	logger.Info("worker_event", append(attrs, "event", "execute_start", "phase", job.Phase)...)
	ctx, cancel := context.WithTimeout(parent, 20*time.Minute)
	defer cancel()
	renewDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-renewDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := RenewVideoRenderJobLease(job); err != nil {
					logger.Error("worker_event", append(attrs, "event", "lease_renew_failed", "error_type", fmt.Sprintf("%T", err))...)
				}
			}
		}
	}()
	defer close(renewDone)

	timeline, err := parseVideoRenderTimeline(job)
	if err != nil {
		failVideoRender(job, "validation_input", err.Error(), true)
		return
	}
	workDir, err := ensureWorkDir("video-render-")
	if err != nil {
		failVideoRender(job, "internal", err.Error(), false)
		return
	}
	defer os.RemoveAll(workDir)

	if err := repository.AdvanceVideoRenderPhase(job.OrganizationID, job.ID, job.LeaseToken, "materializing", model.VideoRenderStatusMaterializing, now()); err != nil {
		failVideoRender(job, "internal", err.Error(), false)
		return
	}
	shotFiles, bgmFile, err := materializeVideoRenderAssets(ctx, job.OrganizationID, workDir, timeline)
	if err != nil {
		failVideoRender(job, "storage_archive", err.Error(), false)
		return
	}
	if err := repository.AdvanceVideoRenderPhase(job.OrganizationID, job.ID, job.LeaseToken, "rendering", model.VideoRenderStatusRendering, now()); err != nil {
		failVideoRender(job, "internal", err.Error(), false)
		return
	}
	outputPath := filepath.Join(workDir, "output.mp4")
	args, err := buildVideoRenderFFmpegArgs(workDir, timeline, shotFiles, bgmFile, outputPath)
	if err != nil {
		failVideoRender(job, "validation_input", err.Error(), true)
		return
	}
	if err := runVideoRenderFFmpeg(ctx, args); err != nil {
		failVideoRender(job, "upstream_permanent", err.Error(), true)
		return
	}
	info, err := os.Stat(outputPath)
	if err != nil || info.Size() < 1 {
		failVideoRender(job, "upstream_permanent", "ffmpeg produced empty output", true)
		return
	}
	if err := repository.AdvanceVideoRenderPhase(job.OrganizationID, job.ID, job.LeaseToken, "archiving", model.VideoRenderStatusArchiving, now()); err != nil {
		failVideoRender(job, "internal", err.Error(), false)
		return
	}
	storageKey, err := archiveVideoRenderResult(ctx, job, outputPath, info.Size())
	if err != nil {
		failVideoRender(job, "storage_archive", err.Error(), false)
		return
	}
	actual := job.EstimatedCredits
	if err := repository.FinishVideoRenderJobSuccess(job.OrganizationID, job.ID, job.LeaseToken, storageKey, info.Size(), actual, now()); err != nil {
		failVideoRender(job, "internal", err.Error(), false)
		return
	}
	logger.Info("worker_event", append(attrs, "event", "execute_success", "result_storage_key", storageKey, "result_size", info.Size())...)
}

func failVideoRender(job model.VideoRenderJob, category, message string, terminal bool) {
	slog.Default().Error("worker_event", "component", "worker", "worker", "video_render", "event", "execute_failed", "job_id", job.ID, "error_category", category, "error_type", category)
	_ = repository.FinishVideoRenderJobFailure(job.OrganizationID, job.ID, job.LeaseToken, category, truncateVideoRenderError(message), videoRenderNextAttemptAt(job.Attempt), now(), terminal)
}

func truncateVideoRenderError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 500 {
		return message[:500]
	}
	return message
}

func materializeVideoRenderAssets(ctx context.Context, organizationID, workDir string, timeline model.VideoTimeline) (map[string]string, string, error) {
	shotFiles := map[string]string{}
	for _, shot := range timeline.Shots {
		path, err := downloadOrganizationFileTo(ctx, organizationID, shot.Source.StorageKey, workDir, "shot-"+shot.ID)
		if err != nil {
			return nil, "", err
		}
		shotFiles[shot.ID] = path
	}
	bgmFile := ""
	if timeline.BGM != nil && timeline.BGM.StorageKey != "" {
		path, err := downloadOrganizationFileTo(ctx, organizationID, timeline.BGM.StorageKey, workDir, "bgm")
		if err != nil {
			return nil, "", err
		}
		bgmFile = path
	}
	return shotFiles, bgmFile, nil
}

func downloadOrganizationFileTo(ctx context.Context, organizationID, storageKey, workDir, basename string) (string, error) {
	item, ok, err := repository.GetUserFile(organizationID, storageKey)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("file missing: %s", storageKey)
	}
	ext := filepath.Ext(item.ObjectKey)
	if ext == "" {
		ext = assetFileExt(item.MimeType)
	}
	dest := filepath.Join(workDir, basename+ext)
	setting, err := currentUserStorageSetting()
	if err != nil {
		return "", err
	}
	if effectiveStorageDriver(item.StorageDriver) == "local" {
		srcPath, err := localStorageObjectPath(setting, item.ObjectKey)
		if err != nil {
			return "", err
		}
		in, err := os.Open(srcPath)
		if err != nil {
			return "", err
		}
		defer in.Close()
		out, err := os.Create(dest)
		if err != nil {
			return "", err
		}
		defer out.Close()
		if _, err := io.Copy(out, in); err != nil {
			return "", err
		}
		return dest, nil
	}
	url, ok := organizationFileURL(organizationID, storageKey, "", 30*time.Minute)
	if !ok {
		return "", errors.New("cannot sign storage url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download status %d", resp.StatusCode)
	}
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", err
	}
	return dest, nil
}

func archiveVideoRenderResult(ctx context.Context, job model.VideoRenderJob, path string, size int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	user := model.AuthUser{ID: job.CreatedBy, OrganizationID: job.OrganizationID}
	archived, err := ArchiveGeneratedFile(ctx, user, job.ID, "video", 0, "video/mp4", size, file)
	if err != nil {
		return "", err
	}
	_ = repository.SetUserFileReferences(job.OrganizationID, "video_render_result", job.ID, "video-render-"+job.ID, []string{archived.StorageKey}, now())
	return archived.StorageKey, nil
}
