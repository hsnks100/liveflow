package pipe

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

type ProcessInterface[T any, U any] interface {
	SetTimeout(timeout time.Duration)
	Link(target func(U))
	GenTarget(processFunc func(T) (U, error), initFunc func() error) func(T)
	Process(data T) (U, error)
	Init() error
}

// BaseProcess struct that provides common functionality
type BaseProcess[T any, U any] struct {
	nextTargets []func(U)
	timeout     time.Duration
	initialized bool
	initOnce    sync.Once
}

func (bp *BaseProcess[T, U]) SetTimeout(timeout time.Duration) {
	bp.timeout = timeout
}

func (bp *BaseProcess[T, U]) Link(target func(U)) {
	bp.nextTargets = append(bp.nextTargets, target)
}

func (bp *BaseProcess[T, U]) GenTarget(processFunc func(T) (U, error), initFunc func() error) func(T) {
	return func(data T) {
		var err error
		bp.initOnce.Do(func() {
			err = initFunc() // Initialize only once
		})
		if err != nil {
			fmt.Println(err)
			return
		}
		resultChan := make(chan U, 1)
		errChan := make(chan error, 1)
		go func() {
			result, err := processFunc(data)
			if err != nil {
				errChan <- err
				return
			}
			resultChan <- result
		}()

		select {
		case processedData := <-resultChan:
			//fmt.Println(processedData)
			for _, target := range bp.nextTargets {
				if target != nil {
					target(processedData)
				}
			}
		case err := <-errChan:
			fmt.Println(err)
		case <-time.After(bp.timeout):
			fmt.Println("Timeout occurred")
		}
	}
}

// PipeExecutor struct for executing the pipeline
type PipeExecutor[T any] struct {
	start            func(T)
	lastFlow         time.Time
	mu               sync.Mutex
	stopChan         chan struct{}
	idleTimeout      time.Duration
	startMonitorOnce sync.Once
}

func NewPipeExecutor[T any, U any](starter ProcessInterface[T, U], idleTimeout time.Duration) *PipeExecutor[T] {
	start := MakeStarter(starter)
	executor := &PipeExecutor[T]{
		start:            start,
		idleTimeout:      idleTimeout,
		stopChan:         make(chan struct{}),
		startMonitorOnce: sync.Once{},
	}
	return executor
}

func (pe *PipeExecutor[T]) Execute(data T) {
	pe.startMonitorOnce.Do(func() {
		go pe.startMonitoring()
	})

	pe.mu.Lock()
	pe.lastFlow = time.Now()
	pe.mu.Unlock()
	pe.start(data)
}

func (pe *PipeExecutor[T]) startMonitoring() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			pe.mu.Lock()
			if time.Since(pe.lastFlow) > pe.idleTimeout {
				fmt.Println("lastFlow", pe.lastFlow, "idleTimeout", pe.idleTimeout)
				fmt.Println("No data flow detected in the pipeline for", pe.idleTimeout)
				pe.lastFlow = time.Now() // Reset lastFlow to avoid repeated logs
				pe.StopMonitoring()
			}
			pe.mu.Unlock()
		case <-pe.stopChan:
			return
		}
	}
}

func (pe *PipeExecutor[T]) StopMonitoring() {
	close(pe.stopChan)
}

type AProcess struct {
	BaseProcess[int, float64]
}

func (a *AProcess) Init() {
	fmt.Println("AProcess initialized")
}

func (a *AProcess) Process(data int) float64 {
	time.Sleep(50 * time.Millisecond) // Simulating processing time
	return float64(data) + 0.5
}

// BProcess struct (float64 -> string)
type BProcess struct {
	BaseProcess[float64, string]
}

func (b *BProcess) Init() {
	fmt.Println("BProcess initialized")
}

func (b *BProcess) Process(data float64) string {
	time.Sleep(100 * time.Millisecond) // Simulating processing time
	return strconv.FormatFloat(data, 'f', 2, 64)
}

// CProcess struct (string -> string)
type CProcess struct {
	BaseProcess[string, string]
}

func (c *CProcess) Init() {
	fmt.Println("CProcess initialized")
}

func (c *CProcess) Process(data string) string {
	time.Sleep(150 * time.Millisecond) // Simulating processing time
	return data + " processed"
}

func LinkProcesses[T any, U any, V any](a ProcessInterface[T, U], b ProcessInterface[U, V]) {
	a.Link(b.GenTarget(b.Process, b.Init))
}

func MakeStarter[T any, U any](start ProcessInterface[T, U]) func(T) {
	return start.GenTarget(start.Process, start.Init)
}
