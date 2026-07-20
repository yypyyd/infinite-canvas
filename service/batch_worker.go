package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

type BatchProductionExecution struct {
	Job     model.BatchProductionJob  `json:"job"`
	Item    model.BatchProductionItem `json:"item"`
	Brand   *model.Brand               `json:"brand,omitempty"`
	Product model.Product             `json:"product"`
	SKU     *model.ProductSKU          `json:"sku,omitempty"`
}

type BatchProductionResult struct {
	ResultURL string `json:"resultUrl"`
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
	body, err := json.Marshal(input)
	if err != nil { return BatchProductionResult{}, err }
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, executor.URL, bytes.NewReader(body))
	if err != nil { return BatchProductionResult{}, err }
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", fmt.Sprintf("%s:%d", input.Item.ID, input.Item.RunNumber))
	if strings.TrimSpace(executor.Token) != "" { request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(executor.Token)) }
	client := executor.Client
	if client == nil { client = &http.Client{Timeout: 15 * time.Minute} }
	response, err := client.Do(request)
	if err != nil { return BatchProductionResult{}, err }
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil { return BatchProductionResult{}, err }
	if response.StatusCode < 200 || response.StatusCode >= 300 { return BatchProductionResult{}, fmt.Errorf("executor returned HTTP %d", response.StatusCode) }
	var result BatchProductionResult
	if err := json.Unmarshal(data, &result); err != nil { return result, err }
	result.ResultURL = strings.TrimSpace(result.ResultURL)
	if !validHTTPURL(result.ResultURL) { return result, errors.New("executor returned invalid result URL") }
	return result, nil
}

func RunBatchProductionWorker(ctx context.Context, concurrency int, executor BatchProductionExecutor) error {
	if executor == nil { return errors.New("batch production executor is required") }
	if concurrency < 1 { concurrency = 1 }
	var workers sync.WaitGroup
	workers.Add(concurrency)
	for index := 0; index < concurrency; index++ {
		go func() {
			defer workers.Done()
			batchProductionWorkerLoop(ctx, executor)
		}()
	}
	<-ctx.Done()
	workers.Wait()
	return nil
}

func batchProductionWorkerLoop(ctx context.Context, executor BatchProductionExecutor) {
	for ctx.Err() == nil {
		item, job, claimed, err := ClaimBatchProductionItem()
		if err != nil {
			log.Printf("claim batch production item failed: %v", err)
			waitBatchWorker(ctx, 2*time.Second)
			continue
		}
		if !claimed {
			waitBatchWorker(ctx, 2*time.Second)
			continue
		}
		executeBatchProductionItem(ctx, executor, item, job)
	}
}

func executeBatchProductionItem(parent context.Context, executor BatchProductionExecutor, item model.BatchProductionItem, job model.BatchProductionJob) {
	execution, err := batchProductionExecution(item, job)
	if err != nil {
		_ = FinishBatchProductionItem(item, false, "", err.Error())
		return
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	leaseDone := make(chan struct{})
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
				if err := RenewBatchProductionItemLease(item); err != nil { cancel(); return }
			}
		}
	}()
	result, executeErr := executor.Execute(ctx, execution)
	close(leaseDone)
	if parent.Err() != nil { return }
	if executeErr != nil {
		if err := FinishBatchProductionItem(item, false, "", executeErr.Error()); err != nil { log.Printf("finish failed batch production item failed item=%s err=%v", item.ID, err) }
		return
	}
	if err := FinishBatchProductionItem(item, true, result.ResultURL, ""); err != nil { log.Printf("finish batch production item failed item=%s err=%v", item.ID, err) }
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
	return execution, nil
}

func batchProductionErrorMessage(message string) string {
	message = strings.TrimSpace(strings.Map(func(value rune) rune { if unicode.IsControl(value) { return ' ' }; return value }, message))
	characters := []rune(message)
	if len(characters) > 1000 { message = string(characters[:1000]) }
	return message
}

func validHTTPURL(value string) bool {
	if len(strings.TrimSpace(value)) > 4096 { return false }
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && parsed.User == nil
}

func waitBatchWorker(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select { case <-ctx.Done(): case <-timer.C: }
}
