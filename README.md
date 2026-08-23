# task191-reproof — 构建图可复现性隔离证明服务

构建基础设施工程师登记构建动作 DAG、声明输入输出与工具链，写入观测访问日志；
服务将实际读取/写入与声明边比对，追踪污染产物与非确定性来源，计算每个目标的
可复现证明或最短违规链；通过的目标可发布为冻结输入哈希的可复现基线，
后续日志作为新验证轮次与基线比对，漂移即失效证明。

## 业务闭环

1. 创建构建目标，登记动作 DAG（依赖边，拒绝环）。
2. 登记种子输入（外部提供，无需动作生产）、动作的输入/输出声明与工具链版本。
3. 批量写入观测访问日志（按 `(action, seq)` 合并，冲突内容保留）。
4. 运行隔离性分析：读取泄漏、未声明写入、缺失工具链、读取不存在输入、
   无序写冲突、DAG 环，并计算最短违规链。
5. 无违规且全部动作已观测 → 目标 `proven`，生成有效证明（图哈希 + 日志哈希）。
6. 发布基线：冻结全部输入路径哈希，目标 `baselined`。
7. 后续日志轮次与基线比对，哈希漂移 → 证明失效、目标退回 `building`。

## 核心状态机

- 目标：`building → proven / irreproducible / insufficient → baselined`
- 动作：`pending → declared → verified / read_leak / write_conflict`
- 输入/输出：`declared → observed → pinned`；未声明访问 → `undeclared/polluted`
- 证明：`draft → valid → invalidated / superseded`

## 标准命令

```bash
# 构建（固定工具链，纯 Go SQLite 无需 CGO）
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
# 静态检查
CGO_ENABLED=0 GOTOOLCHAIN=local go vet ./...
# 测试
CGO_ENABLED=0 GOTOOLCHAIN=local go test ./...
# 端到端自检（不启动服务，完成即退出 0）
go run ./cmd/reproof --smoke-test
# 启动服务
go run ./cmd/reproof --addr :8090 --db reproof.db
```

## API 入口（前缀 /api）

| 能力 | API |
| --- | --- |
| 目标 | `POST/GET /api/targets`、`GET /api/targets/{id}` |
| 种子输入 | `POST/GET /api/targets/{id}/seeds` |
| 动作 | `POST /api/targets/{id}/actions`、`GET /api/targets/{id}/actions`、`GET /api/actions/{id}` |
| 依赖边 | `POST /api/actions/{id}/deps` |
| 声明 | `POST /api/actions/{id}/declarations`、`GET /api/targets/{id}/declarations` |
| 工具链 | `POST /api/actions/{id}/toolchains` |
| 访问日志 | `POST /api/actions/{id}/logs`、`GET /api/actions/{id}/logs` |
| 分析 | `POST /api/targets/{id}/analyze`、`GET /api/targets/{id}/violations`、`GET /api/targets/{id}/chains`、`GET /api/targets/{id}/pollution` |
| 证明 | `POST/GET /api/targets/{id}/proofs`、`GET /api/proofs/{id}`、`POST /api/proofs/{id}/invalidate` |
| 基线 | `POST/GET /api/targets/{id}/baseline`、`GET /api/targets/{id}/baselines`、`POST /api/targets/{id}/compare` |
| 系统 | `GET /api/stats`、`GET /api/health` |

## 持久化

SQLite（modernc.org/sqlite 纯 Go，CGO_ENABLED=0 可构建），表：`targets`、
`actions`、`action_deps`、`declarations`、`toolchains`、`access_logs`、
`artifacts`、`artifact_readers`、`violations`、`violation_chains`、`proofs`、
`baselines`、`baseline_items`、`target_seeds`。重启后从未分析目标恢复；
相同图与日志哈希复用证明结论；基线冻结全部输入哈希。
