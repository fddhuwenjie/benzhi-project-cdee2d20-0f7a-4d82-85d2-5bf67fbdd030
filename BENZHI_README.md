# BENZHI_README

## 项目说明
- 项目：benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030
- 项目用途：面向科研洞穴潜水团队的任务安全放行 HTTP 服务，完整实现任务草案、洞段风险、生命支持方案、装备证据、应急演练整改、独立监督签发、不可变归档与审计查询闭环。
- Go 工具链：`golang:1.25`
- 前端工具链：无

## 项目描述
- 项目名称：洞潜任务安全放行服务
- 项目介绍：面向科研洞穴潜水团队的任务安全放行 HTTP 服务，将任务建档、风险研判、人员与呼吸气体方案审核、装备核验、演练整改、监督签发和不可变归档收束为一条可追溯闭环。
- 项目概述：面向科研洞穴潜水团队的任务安全放行 HTTP 服务，将任务建档、风险研判、人员与呼吸气体方案审核、装备核验、演练整改、监督签发和不可变归档收束为一条可追溯闭环。
- 核心工作流：任务负责人创建洞潜任务草案并提交分段风险，审核通过人员与呼吸气体方案后由核验员登记装备证据；团队完成失联与气体共享演练，若存在偏差则进入整改并定向复验，全部门禁满足后由独立监督员签发放行，最终生成不可变任务档案并关闭流程。
- 对外接口：Go 服务提供版本化 HTTP JSON API，所有写请求携带 request_id 与 expected_revision；监听地址支持 -addr=127.0.0.1:<port>，默认 127.0.0.1:19081，并支持以 PORT 端口号绑定 127.0.0.1:<PORT>，绝不默认绑定常见低位端口或 0.0.0.0。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/server -self-check -addr=127.0.0.1:19081

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030-arm64 linux/arm64

docker run -it benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -self-check -addr=127.0.0.1:19081`
