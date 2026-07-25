package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
	"github.com/qiniu/go-sdk/v7/client"
	"github.com/qiniu/go-sdk/v7/storage"
)

type BatchProductionExecution struct {
	Job     model.BatchProductionJob  `json:"job"`
	Item    model.BatchProductionItem `json:"item"`
	Brand   *model.Brand               `json:"brand,omitempty"`
	Product model.Product             `json:"product"`
	SKU     *model.ProductSKU          `json:"sku,omitempty"`
	MediaURLs map[string]string        `json:"mediaUrls,omitempty"`
}

type BatchProductionResult struct {
	ResultURL     string                `json:"resultUrl"`
	MimeType      string                `json:"mimeType"`
	Size          int64                 `json:"size"`
	Data          []byte                `json:"-"`
	GenerationTask *model.GenerationTask `json:"-"`
}

type BatchProductionExecutor interface {
	Execute(context.Context, BatchProductionExecution) (BatchProductionResult, error)
}

type HTTPBatchProductionExecutor struct {
	URL    string
	Token  string
	Client *http.Client
}

func (executor HTTPBatchProductionExecutor) Execute(ctx context.Context, input BatchProductionExecution) (BatchProductionResult, error) {
	if !validHTTPSURL(executor.URL) { return BatchProductionResult{}, errors.New("batch production executor URL must use HTTPS") }
	body, err := json.Marshal(input)
	if err != nil { return BatchProductionResult{}, err }
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, executor.URL, bytes.NewReader(body))
	if err != nil { return BatchProductionResult{}, err }
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", fmt.Sprintf("%s:%d", input.Item.ID, input.Item.RunNumber))
	if strings.TrimSpace(executor.Token) != "" { request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(executor.Token)) }
	client := http.Client{}
	if executor.Client != nil { client = *executor.Client }
	if client.Timeout <= 0 || client.Timeout > 15*time.Minute { client.Timeout = 15 * time.Minute }
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil { return BatchProductionResult{}, err }
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil { return BatchProductionResult{}, err }
	if response.StatusCode < 200 || response.StatusCode >= 300 { return BatchProductionResult{}, fmt.Errorf("executor returned HTTP %d", response.StatusCode) }
	var result BatchProductionResult
	if err := json.Unmarshal(data, &result); err != nil { return result, err }
	result.ResultURL = strings.TrimSpace(result.ResultURL)
	if !validHTTPSURL(result.ResultURL) { return result, errors.New("executor returned invalid result URL") }
	result.MimeType = strings.TrimSpace(strings.Split(result.MimeType, ";")[0])
	if result.Size <= 0 || result.Size > maxUserFileSize || assetTypeFromMime(result.MimeType) == "" { return result, errors.New("executor returned invalid result metadata") }
	return result, nil
}

func RunBatchProductionWorker(ctx context.Context, concurrency int, tenantConcurrency int, executor BatchProductionExecutor) error {
	if executor == nil { return errors.New("batch production executor is required") }
	if concurrency < 1 { concurrency = 1 }
	if tenantConcurrency < 1 { tenantConcurrency = 1 }
	if tenantConcurrency > concurrency { tenantConcurrency = concurrency }
	logWorkerInfo("batch_production", "worker_started", "concurrency", concurrency, "tenant_concurrency", tenantConcurrency)
	var workers sync.WaitGroup
	workers.Add(concurrency)
	for index := 0; index < concurrency; index++ {
		go func() {
			defer workers.Done()
			batchProductionWorkerLoop(ctx, tenantConcurrency, executor)
		}()
	}
	<-ctx.Done()
	workers.Wait()
	logWorkerInfo("batch_production", "worker_stopped")
	return nil
}

func batchProductionWorkerLoop(ctx context.Context, maxTenantRunning int, executor BatchProductionExecutor) {
	for ctx.Err() == nil {
		item, job, claimed, err := ClaimBatchProductionItem(maxTenantRunning)
		if err != nil {
			logWorkerError("batch_production", "item_claim_failed", err)
			waitBatchWorker(ctx, 2*time.Second)
			continue
		}
		if !claimed {
			waitBatchWorker(ctx, 2*time.Second)
			continue
		}
		logWorkerInfo("batch_production", "item_claimed", batchProductionLogAttrs(item, job)...)
		executeBatchProductionItem(ctx, executor, item, job)
	}
}

func executeBatchProductionItem(parent context.Context, executor BatchProductionExecutor, item model.BatchProductionItem, job model.BatchProductionJob) {
	startedAt := time.Now()
	logAttrs := batchProductionLogAttrs(item, job)
	execution, err := batchProductionExecution(item, job)
	if err != nil {
		logWorkerError("batch_production", "input_build_failed", err, logAttrs...)
		if finishErr := FinishBatchProductionItem(item, false, "", "批量生产输入不可用"); finishErr != nil { logWorkerError("batch_production", "item_finalize_failed", finishErr, append(logAttrs, "outcome", "invalid_input")...) }
		return
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	leaseDone := make(chan struct{})
	defer close(leaseDone)
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-leaseDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := RenewBatchProductionItemLease(item); err != nil { logWorkerError("batch_production", "lease_renew_failed", err, logAttrs...); cancel(); return }
			}
		}
	}()
	result, executeErr := executor.Execute(ctx, execution)
	if parent.Err() != nil { return }
	if ctx.Err() != nil { return }
	if executeErr != nil {
		settleBatchProductionGeneration(result, false, logAttrs)
		logWorkerError("batch_production", "executor_failed", executeErr, logAttrs...)
		if err := FinishBatchProductionItem(item, false, "", "生产执行器调用失败"); err != nil { logWorkerError("batch_production", "item_finalize_failed", err, append(logAttrs, "outcome", "executor_failed")...) }
		return
	}
	if err := RenewBatchProductionItemLease(item); err != nil { logWorkerError("batch_production", "lease_verify_failed", err, logAttrs...); return }
	result, deliveryErr := prepareProductionDeliveryResult(ctx, result, job.DeliverySpec)
	if deliveryErr != nil {
		settleBatchProductionGeneration(result, false, logAttrs)
		logWorkerError("batch_production", "delivery_transform_failed", deliveryErr, logAttrs...)
		if err := FinishBatchProductionItem(item, false, "", "生产结果交付处理失败"); err != nil { logWorkerError("batch_production", "item_finalize_failed", err, append(logAttrs, "outcome", "delivery_failed")...) }
		return
	}
	storageKey, archiveErr := archiveBatchProductionResult(ctx, item, job, result)
	if ctx.Err() != nil { return }
	if archiveErr != nil {
		settleBatchProductionGeneration(result, false, logAttrs)
		logWorkerError("batch_production", "result_archive_failed", archiveErr, logAttrs...)
		if err := FinishBatchProductionItem(item, false, "", "生产结果归档失败"); err != nil { logWorkerError("batch_production", "item_finalize_failed", err, append(logAttrs, "outcome", "archive_failed")...) }
		return
	}
	if err := FinishBatchProductionItem(item, true, storageKey, ""); err != nil {
		logWorkerError("batch_production", "item_finalize_failed", err, append(logAttrs, "outcome", "completed")...)
		return
	}
	settleBatchProductionGeneration(result, true, logAttrs)
	logWorkerInfo("batch_production", "item_completed", append(logAttrs, "duration_ms", time.Since(startedAt).Milliseconds())...)
}

func settleBatchProductionGeneration(result BatchProductionResult, succeeded bool, logAttrs []any) {
	if result.GenerationTask == nil {
		return
	}
	status, message := model.GenerationTaskStatusSuccess, ""
	if !succeeded {
		status, message = model.GenerationTaskStatusFailed, "批量生产未完成"
	}
	if err := FinishGenerationTask(*result.GenerationTask, status, message); err != nil {
		logWorkerError("batch_production", "generation_settlement_failed", err, logAttrs...)
	}
}

func batchProductionLogAttrs(item model.BatchProductionItem, job model.BatchProductionJob) []any {
	return []any{"organization_id", item.OrganizationID, "job_id", job.ID, "item_id", item.ID, "run_number", item.RunNumber, "attempts", item.Attempts}
}

func archiveBatchProductionResult(ctx context.Context, item model.BatchProductionItem, job model.BatchProductionJob, result BatchProductionResult) (string, error) {
	if err := ensureQiniuStorageConfigured(); err != nil { return "", err }
	assetType := assetTypeFromMime(result.MimeType)
	storageKey := fmt.Sprintf("%s:batch-result-%s-%s-%d", assetType, job.ArchiveToken, item.ID, item.RunNumber)
	replaceExisting := false
	replaceObjectKey := ""
	if existing, exists, err := repository.GetUserFile(job.OrganizationID, storageKey); err != nil { return "", err } else if exists {
		info, statErr := qiniuBucketManager().Stat(config.Cfg.QiniuBucket, existing.ObjectKey)
		if statErr == nil {
			if info.Fsize != result.Size || assetTypeFromMime(info.MimeType) != assetType { return "", errors.New("archived batch result conflicts with executor response") }
			if existing.Hash == info.Hash && existing.Hash != "" { return storageKey, nil }
			replaceExisting = true
		} else {
			var qiniuError *client.ErrorInfo
			if !errors.As(statErr, &qiniuError) || qiniuError.Code != 612 { return "", statErr }
			replaceExisting = true
		}
		if replaceExisting { replaceObjectKey = existing.ObjectKey }
	}
	uploadID := newID("batch-result-upload")
	timestamp := now()
	expiresAt := time.Now().UTC().Add(30 * time.Minute)
	reservation := model.UserFileUploadReservation{ID: uploadID, OrganizationID: job.OrganizationID, UserID: job.CreatedBy, StorageKey: storageKey, ObjectKey: batchProductionResultObjectKey(job.OrganizationID, uploadID, result.MimeType), MimeType: result.MimeType, Size: result.Size, ReplaceExisting: replaceExisting, ReplaceObjectKey: replaceObjectKey, ExpiresAt: expiresAt.Format(timestampLayout), CleanupAfter: expiresAt.Add(userFileUploadCleanupGrace).Format(timestampLayout), CreatedAt: timestamp}
	if _, err := repository.ReserveUserFileUpload(reservation, userStorageQuotaBytes(), timestamp); err != nil { return "", err }
	confirmed := false
	defer func() { if !confirmed { _ = repository.CancelUserFileUploadReservation(job.OrganizationID, job.CreatedBy, uploadID, now()) } }()
	var body io.Reader
	var responseBody io.ReadCloser
	if len(result.Data) > 0 {
		if int64(len(result.Data)) != result.Size { return "", errors.New("batch result size does not match executor response") }
		body = bytes.NewReader(result.Data)
	} else {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, result.ResultURL, nil)
		if err != nil { return "", err }
		transport := batchResultTransport()
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 15 * time.Minute, CheckRedirect: func(request *http.Request, via []*http.Request) error { if len(via) >= 5 || !validHTTPSURL(request.URL.String()) { return errors.New("batch result redirect is not allowed") }; return nil }}
		response, err := client.Do(request)
		if err != nil { return "", err }
		responseBody = response.Body
		defer responseBody.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 { return "", fmt.Errorf("batch result returned HTTP %d", response.StatusCode) }
		if response.ContentLength >= 0 && response.ContentLength != result.Size { return "", errors.New("batch result size does not match executor response") }
		responseMime := strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
		if responseMime != "" && assetTypeFromMime(responseMime) != assetType { return "", errors.New("batch result MIME does not match executor response") }
		body = responseBody
	}
	limited := &io.LimitedReader{R: body, N: result.Size + 1}
	policy := storage.PutPolicy{Scope: config.Cfg.QiniuBucket + ":" + reservation.ObjectKey, Expires: 1800, FsizeMin: result.Size, FsizeLimit: result.Size, DetectMime: 1, MimeLimit: assetType + "/*", EndUser: job.CreatedBy}
	var uploaded storage.PutRet
	uploader := storage.NewFormUploader(&storage.Config{UseHTTPS: true})
	if err := uploader.Put(ctx, &uploaded, policy.UploadToken(qiniuMac()), reservation.ObjectKey, limited, result.Size, &storage.PutExtra{MimeType: result.MimeType}); err != nil { return "", err }
	if result.Size+1-limited.N != result.Size { return "", errors.New("batch result size does not match executor response") }
	var extra [1]byte
	if total, _ := body.Read(extra[:]); total > 0 { return "", errors.New("batch result exceeds declared size") }
	info, err := qiniuBucketManager().Stat(config.Cfg.QiniuBucket, reservation.ObjectKey)
	if err != nil { return "", err }
	if info.Fsize != result.Size || assetTypeFromMime(info.MimeType) != assetType { return "", errors.New("archived batch result metadata does not match executor response") }
	file, err := repository.ConfirmUserFileUpload(job.OrganizationID, job.CreatedBy, uploadID, newID("file"), info.Hash, info.MimeType, info.Fsize, userStorageQuotaBytes(), now())
	if err != nil { return "", err }
	confirmed = true
	return file.StorageKey, nil
}

func batchResultTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 30 * time.Second, DisableCompression: true, DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil { return nil, err }
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil { return nil, err }
		var lastErr error
		for _, resolved := range addresses {
			if !publicBatchResultIP(resolved.IP) { continue }
			connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(resolved.IP.String(), port))
			if dialErr == nil { return connection, nil }
			lastErr = dialErr
		}
		if lastErr != nil { return nil, lastErr }
		return nil, errors.New("batch result host resolves to a private address")
	}}
}

func publicBatchResultIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok { return false }
	address = address.Unmap()
	return address.IsGlobalUnicast() && !address.IsPrivate() && !netip.MustParsePrefix("100.64.0.0/10").Contains(address)
}

func batchProductionExecution(item model.BatchProductionItem, job model.BatchProductionJob) (BatchProductionExecution, error) {
	ids := []string{item.ProductSnapshotID}
	if item.BrandSnapshotID != "" { ids = append(ids, item.BrandSnapshotID) }
	if item.SKUSnapshotID != "" { ids = append(ids, item.SKUSnapshotID) }
	snapshots, err := repository.GetBatchProductionSnapshots(item.OrganizationID, ids)
	if err != nil { return BatchProductionExecution{}, err }
	var product model.Product
	if snapshot, ok := snapshots[item.ProductSnapshotID]; !ok || json.Unmarshal([]byte(snapshot.Data), &product) != nil || product.ID != item.ProductID { return BatchProductionExecution{}, errors.New("batch product snapshot is invalid") }
	execution := BatchProductionExecution{Job: job, Item: item, Product: product}
	if item.BrandSnapshotID != "" {
		var brand model.Brand
		if snapshot, ok := snapshots[item.BrandSnapshotID]; !ok || json.Unmarshal([]byte(snapshot.Data), &brand) != nil || brand.ID != job.BrandID { return execution, errors.New("batch brand snapshot is invalid") }
		execution.Brand = &brand
	}
	if item.SKUSnapshotID != "" {
		var sku model.ProductSKU
		if snapshot, ok := snapshots[item.SKUSnapshotID]; !ok || json.Unmarshal([]byte(snapshot.Data), &sku) != nil || sku.ID != item.SKUID || sku.ProductID != item.ProductID { return execution, errors.New("batch SKU snapshot is invalid") }
		execution.SKU = &sku
	}
	storageKeys := []string{}
	if execution.Brand != nil && execution.Brand.LogoStorageKey != "" { storageKeys = append(storageKeys, execution.Brand.LogoStorageKey) }
	if execution.SKU != nil { storageKeys = append(storageKeys, execution.SKU.ImageStorageKeys...) }
	if len(storageKeys) > 0 {
		execution.MediaURLs = make(map[string]string, len(storageKeys))
		for _, storageKey := range storageKeys {
			if _, exists := execution.MediaURLs[storageKey]; exists { continue }
			url, ok := organizationFileURL(item.OrganizationID, storageKey, 30*time.Minute)
			if !ok { return execution, errors.New("batch input media is unavailable") }
			execution.MediaURLs[storageKey] = url
		}
	}
	return execution, nil
}

func batchProductionErrorMessage(message string) string {
	message = strings.TrimSpace(strings.Map(func(value rune) rune { if unicode.IsControl(value) { return ' ' }; return value }, message))
	characters := []rune(message)
	if len(characters) > 1000 { message = string(characters[:1000]) }
	return message
}

func validHTTPSURL(value string) bool {
	if len(strings.TrimSpace(value)) > 4096 { return false }
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func waitBatchWorker(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select { case <-ctx.Done(): case <-timer.C: }
}
