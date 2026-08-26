# SeismoCal地震台站仪器校准放行台

面向地震台站技术人员的浏览器工作台，覆盖校准案件建档、基线冻结、分阶段测量、异常复测、独立复核、证书签发及证据封存。

## 构建、运行与测试

```bash
go test ./...
go run ./cmd/seismocal -addr=127.0.0.1:19081
```

服务默认监听 `127.0.0.1:19081`，也可使用 `-addr` 或 `PORT` 配置。访问 `/ui/calibration` 打开工作台；`-self-check` 会驱动一条完整流程并自动退出。

案件详情还提供环境漂移趋势、阶段测量统计与缺口诊断（`GET /api/v1/cases/{case_id}/measurement-summary`）、证书摘要预览（`GET /api/v1/cases/{case_id}/certificate-preview`）及分层证据清单（`GET /api/v1/cases/{case_id}/evidence-manifest`）。复测通过 `/retest` 受单异常三次上限约束；复核驳回后可通过 `/remediation` 按 `expected_revision` 提交覆盖指定阶段或异常的合格证据，系统会把整改摘要追加到审计哈希链，只有全部整改完成且携带最新 `EvidenceDigest` 才能再次独立复核并继续签发。

已放行案件可在详情页或通过 `GET /api/v1/cases/{case_id}/certificate-verification` 执行只读证书巡检。报告会分别给出证书摘要、审计链与 `audit_head`、证据包摘要及保留期限状态，并列出阻断原因；巡检不会改变案件状态、`revision` 或证书内容。
