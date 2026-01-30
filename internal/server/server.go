package server

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/leon37/neuro-grid/pb"
)

// Task 代表一个具体的推理任务
type Task struct {
	ID         string
	Prompt     string
	ResultChan chan string // 用于把结果传回给调用者 (User)
}

// WorkerNode 扩展之前的 WorkerState
type WorkerNode struct {
	ID          string
	CommandChan chan *Task // Server -> Worker 的指令通道
}

type NeuroServer struct {
	pb.UnimplementedNeuroServiceServer

	mu      sync.RWMutex
	workers map[string]*WorkerNode

	// 全局任务队列 (缓冲 1000 个请求)
	taskQueue chan *Task

	// 信号量：用于优雅退出
	quit chan struct{}
}

func NewNeuroServer() *NeuroServer {
	s := &NeuroServer{
		workers:   make(map[string]*WorkerNode),
		taskQueue: make(chan *Task, 1000), // Backpressure Buffer
		quit:      make(chan struct{}),
	}

	// 启动调度主循环 (The Game Loop)
	go s.scheduleLoop()
	return s
}

// 核心调度循环：相当于游戏里的 Update() / Tick()
func (s *NeuroServer) scheduleLoop() {
	log.Println("[Scheduler] Logic loop started...")

	for {
		select {
		case task := <-s.taskQueue:
			// 1. 收到任务，寻找空闲 Worker
			worker := s.pickIdleWorker()

			if worker != nil {
				// 2. 派发任务 (非阻塞，防止 Worker 假死拖累 Server)
				select {
				case worker.CommandChan <- task:
					log.Printf("[Dispatch] Task %s -> Worker %s", task.ID, worker.ID)
				default:
					log.Printf("[Drop] Worker %s buffer full, task %s dropped", worker.ID, task.ID)
					// 生产环境这里应该重新入队 (Re-queue)
				}
			} else {
				// 3. 无可用 Worker，暂时丢弃或排队
				// 简单起见，这里直接报错 (或者你可以实现一个 WaitingQueue)
				log.Printf("[Overflow] No idle workers for task %s", task.ID)
				// 通知用户失败
				close(task.ResultChan)
			}

		case <-s.quit:
			return
		}
	}
}

// pickIdleWorker: 简单的负载均衡策略 (Random or First-Available)
func (s *NeuroServer) pickIdleWorker() *WorkerNode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 简单轮询：找到第一个存在的 Worker (实际场景应该配合 Status 字段)
	for _, w := range s.workers {
		return w
	}
	return nil
}

// SubmitTask: 供外部 (gRPC Handler) 调用
func (s *NeuroServer) SubmitTask(prompt string) (<-chan string, error) {
	resultChan := make(chan string, 10) // 缓冲 Token 流

	select {
	case s.taskQueue <- &Task{
		ID:         fmt.Sprintf("req-%d", time.Now().UnixNano()),
		Prompt:     prompt,
		ResultChan: resultChan,
	}:
		return resultChan, nil
	default:
		return nil, errors.New("system overloaded")
	}
}
