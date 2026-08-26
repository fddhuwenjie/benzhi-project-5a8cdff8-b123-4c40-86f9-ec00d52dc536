# BENZHI_README

## 项目说明
- 项目：benzhi-project-5a8cdff8-b123-4c40-86f9-ec00d52dc536
- 项目用途：SeismoCal地震台站仪器校准放行台提供案件建档、基线冻结、分阶段测量、异常复测、独立复核、数字证书签发和不可变证据封存的完整浏览器工作流。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 项目描述
- 项目名称：SeismoCal地震台站仪器校准放行台
- 项目介绍：面向地震台站技术人员的浏览器工作台，用于把一台地震观测仪器从校准案件建档、分阶段测量、异常复测、独立复核推进到数字证书签发和不可变证据封存，形成可追溯的单一业务闭环。
- 项目概述：面向地震台站技术人员的浏览器工作台，用于把一台地震观测仪器从校准案件建档、分阶段测量、异常复测、独立复核推进到数字证书签发和不可变证据封存，形成可追溯的单一业务闭环。
- 核心工作流：地震观测仪器校准放行：建档并冻结基线，登记环境与分阶段测量，质量门禁发现异常后暂停并完成复测，提交独立复核意见，签发校准证书并封存证据包。
- 对外接口：Go 服务提供原生 HTML、CSS、JavaScript 的校准放行页面；页面通过同源版本化 JSON API 创建案件、录入测量、处理异常、提交复核、签发证书和查看只读封存时间线，无需 Node 构建链。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/seismocal -self-check -addr=127.0.0.1:19081

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-5a8cdff8-b123-4c40-86f9-ec00d52dc536-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-5a8cdff8-b123-4c40-86f9-ec00d52dc536-arm64 linux/arm64

docker run -it benzhi-project-5a8cdff8-b123-4c40-86f9-ec00d52dc536-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/seismocal -self-check -addr=127.0.0.1:19081`
