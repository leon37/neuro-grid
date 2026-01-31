package server

import (
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/leon37/neuro-grid/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Task 结构体：在内部传递的任务包
type Task struct {
	ID         string
	Prompt     string
	ResultChan chan<- string // 只写通道
}

// WorkerNode: 现在的 Worker 是一个有状态的管道
type WorkerNode struct {
	ID          string
	Stream      pb.NeuroService_WorkerStreamServer // 原始 gRPC 流句柄
	CommandChan chan *pb.ServerCommand             // 发送队列
	Quit        chan struct{}
}

type NeuroServer struct {
	pb.UnimplementedNeuroServiceServer

	mu        sync.RWMutex
	workers   map[string]*WorkerNode
	taskQueue chan *Task
}

func NewNeuroServer() *NeuroServer {
	return &NeuroServer{
		workers:   make(map[string]*WorkerNode),
		taskQueue: make(chan *Task, 1000), // Buffer
	}
}

// 核心接口: WorkerStream
// 这是一个长连接，直到 Worker 断开前都不会 return
func (s *NeuroServer) WorkerStream(stream pb.NeuroService_WorkerStreamServer) error {
	// 1. 握手阶段 (Handshake)
	// 强制 Worker 发送的第一条消息必须是 Ping (含 ID)
	firstMsg, err := stream.Recv()
	if err != nil {
		return err
	}

	workerID := ""
	// 解析 Oneof 字段
	switch payload := firstMsg.Payload.(type) {
	case *pb.WorkerData_Ping:
		workerID = payload.Ping.WorkerId
	default:
		return status.Errorf(codes.Unauthenticated, "First message must be Ping")
	}

	log.Printf("[Synapse] Worker %s connected via Neural Link.", workerID)

	// 2. 注册节点 (Register)
	node := &WorkerNode{
		ID:          workerID,
		Stream:      stream,
		CommandChan: make(chan *pb.ServerCommand, 100),
		Quit:        make(chan struct{}),
	}

	s.mu.Lock()
	s.workers[workerID] = node
	s.mu.Unlock()

	// 3. 启动发送协程 (The Sender)
	// 专门负责把 CommandChan 里的东西写进 gRPC Stream
	go s.handleSendLoop(node)

	// 4. 清理闭包 (Cleanup)
	defer func() {
		s.mu.Lock()
		delete(s.workers, workerID)
		s.mu.Unlock()
		close(node.Quit) // 通知发送协程退出
		log.Printf("[Disconnect] Worker %s lost signal.", workerID)
	}()

	// 5. 接收主循环 (The Receiver)
	// 阻塞在这里，直到连接断开
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil // 正常关闭
		}
		if err != nil {
			return err // 异常断开
		}

		// 处理接收到的数据
		switch payload := msg.Payload.(type) {
		case *pb.WorkerData_Ping:
			// 处理心跳，这里简单回复一个 Pong
			select {
			case node.CommandChan <- &pb.ServerCommand{
				Payload: &pb.ServerCommand_Pong{Pong: &pb.Pong{Ack: true}},
			}:
			default:
				log.Println("Worker send buffer full, dropping pong")
			}

		case *pb.WorkerData_Result:
			// 收到 LLM 生成的 Token！
			// 这里是未来对接 TUI (界面) 的地方
			// TODO: 将 Token 路由回原始请求者
			res := payload.Result
			fmt.Printf("\r[%s]: %s", res.RequestId, res.Content) // 简单打印
		}
	}
}

// 发送循环：将 Go Channel 转换为 gRPC Send
func (s *NeuroServer) handleSendLoop(node *WorkerNode) {
	for {
		select {
		case cmd := <-node.CommandChan:
			if err := node.Stream.Send(cmd); err != nil {
				log.Printf("Failed to send to worker %s: %v", node.ID, err)
				return // 发送失败通常意味着连接断了
			}
		case <-node.Quit:
			return // 主接收循环退出了，我也撤
		}
	}
}
