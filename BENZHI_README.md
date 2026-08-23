# BENZHI 评测说明 — task191-reproof

## 项目定位

构建图可复现性隔离证明服务：登记构建动作 DAG、声明输入输出、工具链与观测日志，
比对声明边与实际访问，追踪污染产物与非确定性来源，计算可复现证明/最短违规链，
发布冻结输入哈希的基线，后续日志轮次与基线比对。

## 命令

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go test ./...
go run ./cmd/reproof --smoke-test
```

## --smoke-test 契约

`go run ./cmd/reproof --smoke-test` 不启动长驻服务，真实执行：
创建目标 → 建 4 动作 DAG（fetch→compile→bundle→sign）→ 登记种子与声明 →
写入访问日志 → 分析发现 `compile` 读取未声明 `env.local`（读取泄漏）→
补齐声明后重新分析 → 目标 proven → 生成有效证明 → 冻结基线 → 关闭数据库 →
用同一文件重开验证目标/动作/证明/基线恢复、相同图日志哈希复用既有证明 →
追加漂移日志后比对基线发现漂移 → 证明失效、目标退回 building。
全部通过以 0 退出码结束。

## Docker 双架构验证

```bash
bash build_benzhi_docker.sh <镜像名> linux/amd64
bash build_benzhi_docker.sh <镜像名> linux/arm64
docker run --rm <镜像名>:latest   # 默认执行 CMD 即 --smoke-test
```

镜像固定入口 `/app/task191-reproof`，`ENTRYPOINT` + `CMD ["--smoke-test"]`。

## HTTP API

所有能力以 `/api` 前缀暴露，共 25+ 个入口（目标/种子/动作/依赖/声明/工具链/
日志/分析/违规链/污染路径/证明/基线比对/统计/健康），详见 `README.md`。
