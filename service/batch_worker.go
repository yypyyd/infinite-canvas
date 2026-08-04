package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/qiniu/go-sdk/v7/client"
	"github.com/qiniu/go-sdk/v7/storage"
	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

type BatchProductionExecution struct {
	Job       model.BatchProductionJob                `json:"job"`
	Item      model.BatchProductionItem               `json:"item"`
	Selection *model.BatchProductionTemplateSelection `json:"selection,omitempty"`
	Brand     *model.Brand                            `json:"brand,omitempty"`
	Product   model.Product                           `json:"product"`
	SKU       *model.ProductSKU                       `json:"sku,omitempty"`
	MediaURLs map[string]string                       `json:"mediaUrls,omitempty"`
}

type BatchProductionResult struct {
	ResultURL      string                `json:"resultUrl"`
	MimeType       string                `json:"mimeType"`
	Size           int64                 `json:"size"`
	Data           []byte                `json:"-"`
	GenerationTask *model.GenerationTask `json:"-"`
}

type BatchProductionExecutor interface {
	Execute(context.Context, BatchProductionExecution) (BatchProductionResult, error)
}

type batchProductionFailure struct {
	Category  model.BatchProductionErrorCategory
	Retryable bool
	Message   string
}

type batchProductionTypedError struct {
	Category  model.BatchProductionErrorCategory
	Retryable bool
	Cause     error
}

func (err *batchProductionTypedError) Error() string { return err.Cause.Error() }
func (err *batchProductionTypedError) Unwrap() error { return err.Cause }

func permanentBatchProductionError(category model.BatchProductionErrorCategory, cause error) error {
	return &batchProductionTypedError{Category: category, Cause: cause}
}

func transientBatchProductionError(category model.BatchProductionErrorCategory, cause error) error {
	return &batchProductionTypedError{Category: category, Retryable: true, Cause: cause}
}

const maxBatchProductionAttempts = 5

func classifyBatchProductionFailure(err error, fallback model.BatchProductionErrorCategory, message string) batchProductionFailure {
	failure := batchProductionFailure{Category: fallback, Retryable: fallback == model.BatchProductionErrorStorageArchive || fallback == model.BatchProductionErrorInternal, Message: message}
	if err == nil {
		return failure
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		failure.Category, failure.Retryable = model.BatchProductionErrorCancelledLeaseLost, false
		return failure
	}
	var typedError *batchProductionTypedError
	if errors.As(err, &typedError) {
		failure.Category, failure.Retryable = typedError.Category, typedError.Retryable
		return failure
	}
	if errors.Is(err, repository.ErrInsufficientUserCredits) || errors.Is(err, repository.ErrInsufficientOrganizationCredits) || errors.Is(err, repository.ErrOrganizationCreditBudgetExceeded) {
		failure.Category, failure.Retryable = model.BatchProductionErrorPricingCredit, false
		return failure
	}
	value := strings.ToLower(err.Error())
	if strings.Contains(value, "insufficient credits") || strings.Contains(value, "reserved credits") || strings.Contains(value, "pricing") {
		failure.Category, failure.Retryable = model.BatchProductionErrorPricingCredit, false
		return failure
	}
	if strings.Contains(value, "invalid") || strings.Contains(value, "required") || strings.Contains(value, "not configured") {
		failure.Category, failure.Retryable = model.BatchProductionErrorValidationInput, false
		return failure
	}
	if strings.Contains(value, "http 429") || strings.Contains(value, "http 500") || strings.Contains(value, "http 502") || strings.Contains(value, "http 503") || strings.Contains(value, "http 504") {
		failure.Category, failure.Retryable = model.BatchProductionErrorUpstreamTransient, true
		return failure
	}
	if strings.Contains(value, "http 4") {
		failure.Category, failure.Retryable = model.BatchProductionErrorUpstreamPermanent, false
		return failure
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		failure.Category, failure.Retryable = model.BatchProductionErrorUpstreamTransient, true
	}
	return failure
}

func batchProductionNextAttemptAt(attempts int, timestamp time.Time) string {
	delay := 10 * time.Minute
	if attempts <= 1 {
		delay = 30 * time.Second
	} else if attempts == 2 {
		delay = 2 * time.Minute
	}
	return timestamp.UTC().Add(delay).Format(timestampLayout)
}

type HTTPBatchProductionExecutor struct {
	URL    string
	Token  string
	Client *http.Client
}

func (executor HTTPBatchProductionExecutor) Execute(ctx context.Context, input BatchProductionExecution) (BatchProductionResult, error) {
	if !validHTTPSURL(executor.URL) {
		return BatchProductionResult{}, permanentBatchProductionError(model.BatchProductionErrorValidationInput, errors.New("batch production executor URL must use HTTPS"))
	}
	body, err := json.Marshal(input)
	if err != nil {
		return BatchProductionResult{}, transientBatchProductionError(model.BatchProductionErrorInternal, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, executor.URL, bytes.NewReader(body))
	if err != nil {
		return BatchProductionResult{}, permanentBatchProductionError(model.BatchProductionErrorUpstreamPermanent, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", fmt.Sprintf("%s:%d", input.Item.ID, input.Item.RunNumber))
	if strings.TrimSpace(executor.Token) != "" {
		request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(executor.Token))
	}
	client := http.Client{}
	if executor.Client != nil {
		client = *executor.Client
	}
	if client.Timeout <= 0 || client.Timeout > 15*time.Minute {
		client.Timeout = 15 * time.Minute
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return BatchProductionResult{}, transientBatchProductionError(model.BatchProductionErrorUpstreamTransient, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return BatchProductionResult{}, transientBatchProductionError(model.BatchProductionErrorUpstreamTransient, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		statusErr := fmt.Errorf("executor returned HTTP %d", response.StatusCode)
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
			return BatchProductionResult{}, transientBatchProductionError(model.BatchProductionErrorUpstreamTransient, statusErr)
		}
		return BatchProductionResult{}, permanentBatchProductionError(model.BatchProductionErrorUpstreamPermanent, statusErr)
	}
	var result BatchProductionResult
	if err := json.Unmarshal(data, &result); err != nil {
		return result, permanentBatchProductionError(model.BatchProductionErrorUpstreamPermanent, err)
	}
	result.ResultURL = strings.TrimSpace(result.ResultURL)
	if !validHTTPSURL(result.ResultURL) {
		return result, permanentBatchProductionError(model.BatchProductionErrorUpstreamPermanent, errors.New("executor returned invalid result URL"))
	}
	result.MimeType = strings.TrimSpace(strings.Split(result.MimeType, ";")[0])
	if result.Size <= 0 || result.Size > maxUserFileSize || assetTypeFromMime(result.MimeType) == "" {
		return result, permanentBatchProductionError(model.BatchProductionErrorUpstreamPermanent, errors.New("executor returned invalid result metadata"))
	}
	return result, nil
}

func RunBatchProductionWorker(ctx context.Context, concurrency int, tenantConcurrency int, executor BatchProductionExecutor) error {
	if executor == nil {
		return errors.New("batch production executor is required")
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if tenantConcurrency < 1 {
		tenantConcurrency = 1
	}
	if tenantConcurrency > concurrency {
		tenantConcurrency = concurrency
	}
	logWorkerInfo("batch_production", "worker_started", "concurrency", concurrency, "tenant_concurrency", tenantConcurrency)
	var workers sync.WaitGroup
	workers.Add(concurrency)
	for index := 0; index < concurrency; index++ {
		go func(reconcileOwner bool) {
			defer workers.Done()
			batchProductionWorkerLoop(ctx, tenantConcurrency, executor, reconcileOwner)
		}(index == 0)
	}
	<-ctx.Done()
	workers.Wait()
	logWorkerInfo("batch_production", "worker_stopped")
	return nil
}

func batchProductionWorkerLoop(ctx context.Context, maxTenantRunning int, executor BatchProductionExecutor, reconcileOwner bool) {
	nextReconcileAt := time.Time{}
	for ctx.Err() == nil {
		if reconcileOwner && !time.Now().Before(nextReconcileAt) {
			scanned, repaired, rejected, err := repository.ReconcileCompletedBatchGenerationTasks(now(), 100)
			if err != nil {
				logWorkerError("batch_production", "generation_reconcile_failed", err)
			} else {
				logWorkerInfo("batch_production", "generation_reconcile_completed", "scanned", scanned, "repaired", repaired, "rejected", rejected)
			}
			nextReconcileAt = time.Now().Add(30 * time.Second)
		}
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
	finishFailure := func(failure batchProductionFailure, generationWasCharged bool) {
		retryable := failure.Retryable && item.Attempts < maxBatchProductionAttempts
		status, nextAttemptAt := model.BatchProductionStatusFailed, ""
		if retryable {
			status = model.BatchProductionStatusQueued
			nextAttemptAt = batchProductionNextAttemptAt(item.Attempts, time.Now())
		}
		if err := repository.FinishBatchProductionItemWithSchedule(item, status, "", batchProductionErrorMessage(failure.Message), failure.Category, retryable, generationWasCharged, nextAttemptAt, now()); err != nil {
			logWorkerError("batch_production", "item_finalize_failed", err, append(logAttrs, "outcome", failure.Category)...)
		}
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
				if err := RenewBatchProductionItemLease(item); err != nil {
					logWorkerError("batch_production", "lease_renew_failed", err, logAttrs...)
					cancel()
					return
				}
			}
		}
	}()
	execution, err := batchProductionExecution(item, job)
	if err != nil {
		if fenceErr := fenceBatchProductionFailure(parent, ctx, item); fenceErr != nil {
			logWorkerError("batch_production", "input_fence_failed", fenceErr, logAttrs...)
			return
		}
		logWorkerError("batch_production", "input_build_failed", err, logAttrs...)
		finishFailure(classifyBatchProductionFailure(err, model.BatchProductionErrorInternal, "批量生产输入不可用"), false)
		return
	}
	if err := fenceBatchProductionFailure(parent, ctx, item); err != nil {
		logWorkerError("batch_production", "executor_start_fence_failed", err, logAttrs...)
		return
	}
	result, executeErr := executor.Execute(ctx, execution)
	generationSettled := false
	settlementAllowed := true
	settlementTarget := false
	settleGeneration := func(succeeded bool) error {
		if generationSettled {
			return nil
		}
		settlementTarget = succeeded
		if err := settleBatchProductionGeneration(result, item, job, succeeded); err != nil {
			return err
		}
		generationSettled = true
		return nil
	}
	defer func() {
		if generationSettled || !settlementAllowed {
			return
		}
		if err := settleGeneration(settlementTarget); err != nil {
			logWorkerError("batch_production", "generation_settlement_retry_failed", err, logAttrs...)
		}
	}()
	if parent.Err() != nil || ctx.Err() != nil {
		settlementAllowed = false
		return
	}
	if executeErr != nil {
		if err := fenceBatchProductionFailure(parent, ctx, item); err != nil {
			settlementAllowed = false
			logWorkerError("batch_production", "failure_fence_failed", err, logAttrs...)
			return
		}
		if err := settleGeneration(false); err != nil {
			logWorkerError("batch_production", "generation_settlement_failed", err, logAttrs...)
			return
		}
		logWorkerError("batch_production", "executor_failed", executeErr, logAttrs...)
		failure := classifyBatchProductionFailure(executeErr, model.BatchProductionErrorUpstreamPermanent, "生产执行器调用失败")
		finishFailure(failure, result.GenerationTask != nil)
		return
	}
	if err := RenewBatchProductionItemLease(item); err != nil {
		settlementAllowed = false
		logWorkerError("batch_production", "lease_verify_failed", err, logAttrs...)
		return
	}
	if parent.Err() != nil || ctx.Err() != nil {
		settlementAllowed = false
		return
	}
	deliverySpec := job.DeliverySpec
	if execution.Selection != nil {
		deliverySpec = execution.Selection.DeliverySpec
	}
	result, deliveryErr := prepareProductionDeliveryResult(ctx, result, deliverySpec)
	if deliveryErr != nil {
		if err := fenceBatchProductionFailure(parent, ctx, item); err != nil {
			settlementAllowed = false
			logWorkerError("batch_production", "failure_fence_failed", err, logAttrs...)
			return
		}
		if err := settleGeneration(false); err != nil {
			logWorkerError("batch_production", "generation_settlement_failed", err, logAttrs...)
			return
		}
		logWorkerError("batch_production", "delivery_transform_failed", deliveryErr, logAttrs...)
		failure := classifyBatchProductionFailure(deliveryErr, model.BatchProductionErrorStorageArchive, "生产结果交付处理失败")
		finishFailure(failure, result.GenerationTask != nil)
		return
	}
	if parent.Err() != nil || ctx.Err() != nil {
		settlementAllowed = false
		return
	}
	storageKey, archiveErr := archiveBatchProductionResult(ctx, item, job, result)
	if parent.Err() != nil || ctx.Err() != nil {
		settlementAllowed = false
		return
	}
	if archiveErr != nil {
		if err := fenceBatchProductionFailure(parent, ctx, item); err != nil {
			settlementAllowed = false
			logWorkerError("batch_production", "failure_fence_failed", err, logAttrs...)
			return
		}
		if err := settleGeneration(false); err != nil {
			logWorkerError("batch_production", "generation_settlement_failed", err, logAttrs...)
			return
		}
		logWorkerError("batch_production", "result_archive_failed", archiveErr, logAttrs...)
		failure := classifyBatchProductionFailure(archiveErr, model.BatchProductionErrorStorageArchive, "生产结果归档失败")
		finishFailure(failure, result.GenerationTask != nil)
		return
	}
	if err := FinishBatchProductionItem(item, true, storageKey, ""); err != nil {
		settlementAllowed = false
		logWorkerError("batch_production", "item_finalize_failed", err, append(logAttrs, "outcome", "completed")...)
		return
	}
	if err := settleGeneration(true); err != nil {
		logWorkerError("batch_production", "generation_settlement_failed", err, logAttrs...)
		return
	}
	logWorkerInfo("batch_production", "item_completed", append(logAttrs, "duration_ms", time.Since(startedAt).Milliseconds())...)
}

func fenceBatchProductionFailure(parent context.Context, ctx context.Context, item model.BatchProductionItem) error {
	if err := parent.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := RenewBatchProductionItemLease(item); err != nil {
		return err
	}
	if err := parent.Err(); err != nil {
		return err
	}
	return ctx.Err()
}

func settleBatchProductionGeneration(result BatchProductionResult, item model.BatchProductionItem, job model.BatchProductionJob, succeeded bool) error {
	if result.GenerationTask == nil {
		return nil
	}
	task := *result.GenerationTask
	if task.OrganizationID != item.OrganizationID || item.OrganizationID != job.OrganizationID {
		return errors.New("batch production generation task organization association mismatch")
	}
	if task.BatchItemID != item.ID {
		return errors.New("batch production generation task item association mismatch")
	}
	if task.BatchJobID != item.JobID || item.JobID != job.ID {
		return errors.New("batch production generation task job association mismatch")
	}
	if task.RequestID != standardBatchRequestID(item) {
		return errors.New("batch production generation task request association mismatch")
	}
	status, message := model.GenerationTaskStatusSuccess, ""
	if !succeeded {
		status, message = model.GenerationTaskStatusFailed, "批量生产未完成"
	}
	return FinishGenerationTask(task, status, message)
}

func batchProductionLogAttrs(item model.BatchProductionItem, job model.BatchProductionJob) []any {
	return []any{"organization_id", item.OrganizationID, "job_id", job.ID, "item_id", item.ID, "run_number", item.RunNumber, "attempts", item.Attempts}
}

func archiveBatchProductionResult(ctx context.Context, item model.BatchProductionItem, job model.BatchProductionJob, result BatchProductionResult) (string, error) {
	if err := ensureQiniuStorageConfigured(); err != nil {
		return "", permanentBatchProductionError(model.BatchProductionErrorStorageArchive, err)
	}
	assetType := assetTypeFromMime(result.MimeType)
	if assetType == "" || result.Size <= 0 {
		return "", permanentBatchProductionError(model.BatchProductionErrorStorageArchive, errors.New("batch result metadata is invalid"))
	}
	storageKey := fmt.Sprintf("%s:batch-result-%s-%s-%d", assetType, job.ArchiveToken, item.ID, item.RunNumber)
	replaceExisting := false
	replaceObjectKey := ""
	if existing, exists, err := repository.GetUserFile(job.OrganizationID, storageKey); err != nil {
		return "", transientBatchProductionError(model.BatchProductionErrorStorageArchive, err)
	} else if exists {
		info, statErr := qiniuBucketManager().Stat(config.Cfg.QiniuBucket, existing.ObjectKey)
		if statErr == nil {
			if info.Fsize != result.Size || assetTypeFromMime(info.MimeType) != assetType {
				return "", permanentBatchProductionError(model.BatchProductionErrorStorageArchive, errors.New("archived batch result conflicts with executor response"))
			}
			if existing.Hash != "" && existing.Hash != info.Hash {
				return "", permanentBatchProductionError(model.BatchProductionErrorStorageArchive, errors.New("archived batch result hash conflicts with stored metadata"))
			}
			if existing.Hash == info.Hash && existing.Hash != "" {
				return storageKey, nil
			}
			replaceExisting = true
		} else {
			var qiniuError *client.ErrorInfo
			if !errors.As(statErr, &qiniuError) || qiniuError.Code != 612 {
				return "", transientBatchProductionError(model.BatchProductionErrorStorageArchive, statErr)
			}
			replaceExisting = true
		}
		if replaceExisting {
			replaceObjectKey = existing.ObjectKey
		}
	}
	uploadID := newID("batch-result-upload")
	timestamp := now()
	expiresAt := time.Now().UTC().Add(30 * time.Minute)
	reservation := model.UserFileUploadReservation{ID: uploadID, OrganizationID: job.OrganizationID, UserID: job.CreatedBy, StorageKey: storageKey, ObjectKey: batchProductionResultObjectKey(job.OrganizationID, uploadID, result.MimeType), MimeType: result.MimeType, Size: result.Size, ReplaceExisting: replaceExisting, ReplaceObjectKey: replaceObjectKey, ExpiresAt: expiresAt.Format(timestampLayout), CleanupAfter: expiresAt.Add(userFileUploadCleanupGrace).Format(timestampLayout), CreatedAt: timestamp}
	if _, err := repository.ReserveUserFileUpload(reservation, userStorageQuotaBytes(), timestamp); err != nil {
		return "", transientBatchProductionError(model.BatchProductionErrorStorageArchive, err)
	}
	confirmed := false
	defer func() {
		if !confirmed {
			_ = repository.CancelUserFileUploadReservation(job.OrganizationID, job.CreatedBy, uploadID, now())
		}
	}()
	var body io.Reader
	var responseBody io.ReadCloser
	if len(result.Data) > 0 {
		if int64(len(result.Data)) != result.Size {
			return "", permanentBatchProductionError(model.BatchProductionErrorStorageArchive, errors.New("batch result size does not match executor response"))
		}
		body = bytes.NewReader(result.Data)
	} else {
		if !validHTTPSURL(result.ResultURL) {
			return "", permanentBatchProductionError(model.BatchProductionErrorStorageArchive, errors.New("batch result URL is invalid"))
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, result.ResultURL, nil)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return "", err
			}
			return "", permanentBatchProductionError(model.BatchProductionErrorStorageArchive, err)
		}
		transport := batchResultTransport()
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 15 * time.Minute, CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !validHTTPSURL(request.URL.String()) {
				return permanentBatchProductionError(model.BatchProductionErrorStorageArchive, errors.New("batch result redirect is not allowed"))
			}
			return nil
		}}
		response, err := client.Do(request)
		if err != nil {
			var typedError *batchProductionTypedError
			if errors.As(err, &typedError) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return "", err
			}
			return "", transientBatchProductionError(model.BatchProductionErrorStorageArchive, err)
		}
		responseBody = response.Body
		defer responseBody.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			statusErr := fmt.Errorf("batch result returned HTTP %d", response.StatusCode)
			if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
				return "", transientBatchProductionError(model.BatchProductionErrorStorageArchive, statusErr)
			}
			return "", permanentBatchProductionError(model.BatchProductionErrorStorageArchive, statusErr)
		}
		if response.ContentLength >= 0 && response.ContentLength != result.Size {
			return "", permanentBatchProductionError(model.BatchProductionErrorStorageArchive, errors.New("batch result size does not match executor response"))
		}
		responseMime := strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
		if responseMime != "" && assetTypeFromMime(responseMime) != assetType {
			return "", permanentBatchProductionError(model.BatchProductionErrorStorageArchive, errors.New("batch result MIME does not match executor response"))
		}
		body = responseBody
	}
	limited := &io.LimitedReader{R: body, N: result.Size + 1}
	policy := storage.PutPolicy{Scope: config.Cfg.QiniuBucket + ":" + reservation.ObjectKey, Expires: 1800, FsizeMin: result.Size, FsizeLimit: result.Size, DetectMime: 1, MimeLimit: assetType + "/*", EndUser: job.CreatedBy}
	var uploaded storage.PutRet
	uploader := storage.NewFormUploader(&storage.Config{UseHTTPS: true})
	if err := uploader.Put(ctx, &uploaded, policy.UploadToken(qiniuMac()), reservation.ObjectKey, limited, result.Size, &storage.PutExtra{MimeType: result.MimeType}); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		return "", transientBatchProductionError(model.BatchProductionErrorStorageArchive, err)
	}
	if result.Size+1-limited.N != result.Size {
		return "", permanentBatchProductionError(model.BatchProductionErrorStorageArchive, errors.New("batch result size does not match executor response"))
	}
	var extra [1]byte
	if total, readErr := body.Read(extra[:]); readErr != nil && !errors.Is(readErr, io.EOF) {
		return "", transientBatchProductionError(model.BatchProductionErrorStorageArchive, readErr)
	} else if total > 0 {
		return "", permanentBatchProductionError(model.BatchProductionErrorStorageArchive, errors.New("batch result exceeds declared size"))
	}
	info, err := qiniuBucketManager().Stat(config.Cfg.QiniuBucket, reservation.ObjectKey)
	if err != nil {
		return "", transientBatchProductionError(model.BatchProductionErrorStorageArchive, err)
	}
	if info.Fsize != result.Size || assetTypeFromMime(info.MimeType) != assetType {
		return "", permanentBatchProductionError(model.BatchProductionErrorStorageArchive, errors.New("archived batch result metadata does not match executor response"))
	}
	file, err := repository.ConfirmUserFileUpload(job.OrganizationID, job.CreatedBy, uploadID, newID("file"), info.Hash, info.MimeType, info.Fsize, userStorageQuotaBytes(), now())
	if err != nil {
		return "", transientBatchProductionError(model.BatchProductionErrorStorageArchive, err)
	}
	confirmed = true
	return file.StorageKey, nil
}

func batchResultTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 30 * time.Second, DisableCompression: true, DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, resolved := range addresses {
			if !publicBatchResultIP(resolved.IP) {
				continue
			}
			connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(resolved.IP.String(), port))
			if dialErr == nil {
				return connection, nil
			}
			lastErr = dialErr
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, errors.New("batch result host resolves to a private address")
	}}
}

func publicBatchResultIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	return address.IsGlobalUnicast() && !address.IsPrivate() && !netip.MustParsePrefix("100.64.0.0/10").Contains(address)
}

func batchProductionExecution(item model.BatchProductionItem, job model.BatchProductionJob) (BatchProductionExecution, error) {
	ids := []string{item.ProductSnapshotID}
	if item.BrandSnapshotID != "" {
		ids = append(ids, item.BrandSnapshotID)
	}
	if item.SKUSnapshotID != "" {
		ids = append(ids, item.SKUSnapshotID)
	}
	snapshots, err := repository.GetBatchProductionSnapshots(item.OrganizationID, ids)
	if err != nil {
		return BatchProductionExecution{}, transientBatchProductionError(model.BatchProductionErrorInternal, err)
	}
	var product model.Product
	if snapshot, ok := snapshots[item.ProductSnapshotID]; !ok || json.Unmarshal([]byte(snapshot.Data), &product) != nil || product.ID != item.ProductID {
		return BatchProductionExecution{}, permanentBatchProductionError(model.BatchProductionErrorValidationInput, errors.New("batch product snapshot is invalid"))
	}
	execution := BatchProductionExecution{Job: job, Item: item, Product: product}
	if item.TemplateSelectionID != "" {
		selection, ok, err := repository.GetBatchProductionTemplateSelection(item.OrganizationID, item.JobID, item.TemplateSelectionID)
		if err != nil {
			return execution, transientBatchProductionError(model.BatchProductionErrorInternal, err)
		}
		if !ok || selection.TemplateID != item.TemplateID || selection.TemplateVersion != item.TemplateVersion {
			return execution, permanentBatchProductionError(model.BatchProductionErrorValidationInput, errors.New("batch template selection is invalid"))
		}
		execution.Selection = &selection
		execution.Job.PresetID, execution.Job.PresetVersion, execution.Job.PresetPrompt, execution.Job.DeliverySpec = selection.TemplateID, selection.TemplateVersion, selection.Prompt, selection.DeliverySpec
	}
	if item.BrandSnapshotID != "" {
		var brand model.Brand
		if snapshot, ok := snapshots[item.BrandSnapshotID]; !ok || json.Unmarshal([]byte(snapshot.Data), &brand) != nil || brand.ID != job.BrandID {
			return execution, permanentBatchProductionError(model.BatchProductionErrorValidationInput, errors.New("batch brand snapshot is invalid"))
		}
		execution.Brand = &brand
	}
	if item.SKUSnapshotID != "" {
		var sku model.ProductSKU
		if snapshot, ok := snapshots[item.SKUSnapshotID]; !ok || json.Unmarshal([]byte(snapshot.Data), &sku) != nil || sku.ID != item.SKUID || sku.ProductID != item.ProductID {
			return execution, permanentBatchProductionError(model.BatchProductionErrorValidationInput, errors.New("batch SKU snapshot is invalid"))
		}
		execution.SKU = &sku
	}
	storageKeys := []string{}
	if execution.Brand != nil && execution.Brand.LogoStorageKey != "" {
		storageKeys = append(storageKeys, execution.Brand.LogoStorageKey)
	}
	if execution.SKU != nil {
		storageKeys = append(storageKeys, execution.SKU.ImageStorageKeys...)
	}
	if len(storageKeys) > 0 {
		if err := ensureQiniuStorageConfigured(); err != nil {
			return execution, permanentBatchProductionError(model.BatchProductionErrorValidationInput, err)
		}
		execution.MediaURLs = make(map[string]string, len(storageKeys))
		for _, storageKey := range storageKeys {
			if strings.TrimSpace(storageKey) == "" {
				return execution, permanentBatchProductionError(model.BatchProductionErrorValidationInput, errors.New("batch input media storage key is invalid"))
			}
			if _, exists := execution.MediaURLs[storageKey]; exists {
				continue
			}
			file, ok, err := repository.GetUserFile(item.OrganizationID, storageKey)
			if err != nil {
				return execution, transientBatchProductionError(model.BatchProductionErrorInternal, err)
			}
			if !ok || strings.TrimSpace(file.ObjectKey) == "" {
				return execution, permanentBatchProductionError(model.BatchProductionErrorValidationInput, errors.New("batch input media is unavailable"))
			}
			deadline := time.Now().Add(30 * time.Minute).Unix()
			execution.MediaURLs[storageKey] = storage.MakePrivateURL(qiniuMac(), strings.TrimRight(config.Cfg.QiniuDownloadDomain, "/"), file.ObjectKey, deadline)
		}
	}
	return execution, nil
}

func batchProductionErrorMessage(message string) string {
	message = strings.TrimSpace(strings.Map(func(value rune) rune {
		if unicode.IsControl(value) {
			return ' '
		}
		return value
	}, message))
	characters := []rune(message)
	if len(characters) > 1000 {
		message = string(characters[:1000])
	}
	return message
}

func validHTTPSURL(value string) bool {
	if len(strings.TrimSpace(value)) > 4096 {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func waitBatchWorker(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
