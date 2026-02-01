import time
import grpc
import sys
import os
import threading
import uuid
import platform

current_dir = os.path.dirname(os.path.abspath(__file__))
project_root = os.path.dirname(current_dir)
# 这一步是为了让 Python 能找到 proto 目录下的生成代码
sys.path.append(os.path.join(project_root, "proto"))

import agent_pb2
import agent_pb2_grpc

# 配置区
TARGET_IP = "192.168.10.38"  # <---【警告】这里填你 Mac 的 IP！
TARGET_PORT = "8080"
WORKER_ID = f"node-{platform.node()}-{str(uuid.uuid4())[:4]}"


def run():
    # 建立神经连接 (Insecure channel for local dev)
    channel = grpc.insecure_channel(f'{TARGET_IP}:{TARGET_PORT}')
    stub = agent_pb2_grpc.NeuroServiceStub(channel)

    print(f"[*] Connecting to Control Plane at {TARGET_IP}:{TARGET_PORT}...")
    print(f"[*] Identity: {WORKER_ID} | GPU: RTX 4070 Ti (Ready)")

    # 定义双向流生成器
    def stream_generator():
        # 1. 第一帧必须是 Ping (握手)
        print(">>> Sending Handshake (Ping)...")
        yield agent_pb2.WorkerData(
            ping=agent_pb2.Ping(worker_id=WORKER_ID)
        )

        # 2. 进入主循环
        while True:
            # 这里可以插入心跳逻辑，或者等待任务结果
            time.sleep(5)
            # 维持连接活跃 (Optional, 视Server逻辑而定)
            # yield agent_pb2.WorkerData(ping=agent_pb2.Ping(worker_id=WORKER_ID))

    try:
        # 启动双向流
        response_iterator = stub.WorkerStream(stream_generator())

        # 监听来自 Server (Mac) 的指令
        for command in response_iterator:

            if command.HasField('pong'):
                print(f"[+] Heartbeat ACK (Latency: <1ms)")

            elif command.HasField('task'):
                task = command.task
                print(f"\n[!] INCOMING TASK [{task.request_id}]")
                print(f"    Prompt: {task.prompt[:50]}...")
                print("    > Simulating GPU Inference...", end="", flush=True)

                # TODO: 这里后面接 Ollama API
                # 模拟生成过程 (Loading Bar)
                for _ in range(5):
                    time.sleep(0.5)
                    print(".", end="", flush=True)
                print(" Done.")

                # 发送结果回 Server (需要修改 stream_generator 的逻辑，
                # 但 gRPC 的 Python 异步发送比较复杂，
                # 简单起见，我们通常用 Queue 配合 generator 实现，下一步教你)

    except grpc.RpcError as e:
        print(f"\n[!] Connection Lost: {e.code()} - {e.details()}")
    except KeyboardInterrupt:
        print("\n[*] Shutting down...")


if __name__ == '__main__':
    run()