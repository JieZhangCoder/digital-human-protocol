# DHP Registry / Review 部署指南

> 面向第一次接触 Go 的开发者。

---

## 零、当前 Go 服务架构全景

### 0.1 四个角色与两条通路

```
┌─────────────────────────────────────────────────────────────────┐
│                    digital-human-protocol (开源仓)              │
│                                                                 │
│  spec/app-spec.md            ← 协议规范（slug、version、范围）   │
│  packages/                                                      │
│    digital-humans/<slug>/                                       │
│    skills/<author>/<id>/         ← v2 新增 scoped skill        │
│  server/   (Go: 评审 + Registry)                                │
│  .github/workflows/review.yml                                   │
│                                                                 │
│  对外暴露产物 (GitHub Pages / S3 / Go Registry):               │
│    digital-humans.json / skills.json / mcps.json / index.json  │
└─────────────────────────────────────────────────────────────────┘
       ▲                          ▲
       │ GET (静态/mirror sync)   │ POST /apps (publish)
       │                          │
┌──────┴──────────────┐  ┌────────┴────────────────────────┐
│  Halo 客户端          │  │  GitHub Actions                  │
│  (Electron)           │  │  review.yml                      │
│  src/main/store/      │  │   - docker pull ghcr dhp-review │
│   ├ publish/          │  │   - 跑 dhp-review CLI           │
│   ├ dhpkg/            │  │   - 评论 PR / 打标签 / auto-merge │
│   └ upgrade.service   │  └─────────────────────────────────┘
│  src/main/apps/manager│
│  src/renderer/        │
└───────────────────────┘
         │
         │ target=http-registry (企业)
         │ Bearer token
         ▼
┌─────────────────────────────────┐
│  企业 Registry (Go server)      │
│   cmd/registry                  │
│     POST /apps                  │
│     GET  /skills.json 等        │
│     GET  /apps/.../files/*      │
│   internal/rules  (8 条规则)    │
│   internal/ai     (AI judge)    │
│   internal/storage (local)      │
│   (systemd / Docker 可选)       │
└─────────────────────────────────┘
```

### 0.2 模块职责

| 包 | 职责 | 核心类型 |
|---|---|---|
| `internal/config` | 解析 `config.yaml`，提供默认值 | `Config`, `StorageConfig`, `AIJudgeConfig` |
| `internal/storage` | 文件持久化，当前 `local`（磁盘）实现 | `Provider` 接口, `LocalProvider` |
| `internal/ai` | 封装 AI 调用，OpenAI 兼容格式 | `Judge`, `ReviewResult` |
| `internal/rules` | 8 条评审规则，分阻塞型和 AI 型 | `Engine`, `Rule`, `Result`, `Input` |
| `cmd/dhp-review` | CLI 入口，读本地 spec → 输出 JSON 评审结果 | `main.go` |
| `cmd/registry` | HTTP 服务，接收上传 + 提供索引查询 + 文件下载 | `main.go` |

### 0.3 两个入口的区别

| 维度 | `dhp-review` (CLI) | `registry` (HTTP) |
|---|---|---|
| **形态** | 命令行工具 | 常驻后台服务 |
| **调用方** | GitHub Actions、CI 流水线 | hello-halo 客户端、企业用户 |
| **输入** | `--spec path/to/spec.yaml` | `POST /apps` multipart 上传 dhpkg |
| **输出** | 终端 JSON + exit code | HTTP JSON 响应 |
| **是否持久化** | 否（只评审，不存文件） | 是（评审通过则写入 `halo-data/`） |
| **共享代码** | 共用 `rules.Engine` + `ai.Judge` + `config` | 同上 |

### 0.4 与 hello-halo 的交互

hello-halo `feature/app-market-20260514` 分支已实现三个发布 target，Go 服务对应 **`http-registry`** target：

**客户端调用**
```
POST /apps
Content-Type: multipart/form-data
Authorization: Bearer <token>

formData:
  - slug: "openkursar/xhs-search"
  - version: "2.1.0"
  - dhpkg: <zip 文件>
```

**服务端响应**
```json
{
  "slug": "openkursar/xhs-search",
  "version": "2.1.0",
  "verdict": "approved",
  "comment": "All checks passed",
  "results": [...]
}
```

客户端根据 `verdict`：
- `approved` → 提示发布成功
- `rejected` → 展示具体失败规则
- `needs_review` → 提示等待人工审核

**客户端拉取索引**
```
GET /skills.json       → 安装 skill 时拉列表
GET /digital-humans.json → 安装数字人时拉列表
GET /apps/{slug}/{version}/files/{path} → 下载具体文件
```

### 0.5 数据流（一次完整上传）

```
1. 用户点击 hello-halo "发布"按钮
        │
        ▼
2. 客户端打包 dhpkg（spec.yaml + 文件 → zip）
        │
        ▼
3. POST /apps (multipart, Bearer token)
        │
        ▼
4. registry 解压 → 提取 spec → 跑 rules.Engine
        │
        ├── 阻塞规则失败 → 返回 rejected，不存盘
        ├── 需人工审核 → 返回 needs_review，不存盘
        └── 全部通过 → 存盘到 halo-data/，返回 approved
        │
        ▼
5. 其他用户 GET /skills.json → 看到新发布的 skill
        │
        ▼
6. 安装时 GET /apps/{slug}/{version}/files/... → 下载文件
```

### 0.6 评审规则（8 条）

| 规则 | 类型 | 阻塞? | 说明 |
|---|---|---|---|
| `schema_valid` | 模式校验 | 是 | slug、version、name、type 必填 |
| `slug_unique` | 查重 | 是 | 同 slug@version 已存在则拒绝 |
| `dangerous_permissions` | 安全扫描 | 是 | 检测 shell-exec、eval 等高危关键词 |
| `skill_code_safety` | AI 评审 | 是（失败时） | AI 判断代码安全风险 |
| `prompt_quality` | AI 评审 | 否 | AI 判断 prompt 质量 |
| `metadata_compliance` | AI 评审 | 否 | AI 判断元数据合规 |
| `duplicate_detection` | AI 评审 | 否 | AI 检测与已有 skill 重复度 |
| `sensitive_categories` | AI 评审 | 标人工 | AI 判断敏感领域，需人工复核 |

### 0.7 当前限制（已知）

| 点 | 说明 |
|---|---|
| AI judge 未配置时自动跳过 | 所有 AI 规则返回 "auto-approved"，适合开发阶段 |
| 存储只有 local | S3/MinIO 已预留接口，未实现 |
| 无用户体系 | 仅靠 `config.yaml` 里的单一 token 鉴权 |
| 上传未加限流 | 生产环境建议前面加 Nginx 做限流 |
| 索引无缓存 | 每次请求都读磁盘，量大了需要加缓存 |

---

## 一、什么是 Go 编译？

Go 是一门**编译型语言**。

- **开发时**：你看到的是 `.go` 文本文件（人可读的代码）。
- **部署时**：需要把这些文本编译成**二进制可执行文件**（机器直接运行的程序），类似 `.exe`。
- **好处**：编译后**不需要在服务器上安装 Go**，只需要把编译好的二进制文件传过去就能跑。

---

## 二、环境准备

### 1. 安装 Go（编译机）

编译可以在你的 Windows 开发机上做，也可以在 Linux 上做。

**Windows 安装**
1. 下载：https://golang.org/dl/
2. 一路 Next 安装。
3. 验证：打开终端（CMD / PowerShell）
   ```powershell
   go version
   # 输出：go version go1.26.3 windows/amd64
   ```

**Linux 安装（如果你直接在 Linux 上编译）**
```bash
# Ubuntu/Debian
sudo apt-get install golang-go

# CentOS/RHEL
sudo yum install golang
```

### 2. 设置国内镜像源（加快下载依赖）

Go 默认会从国外服务器下载依赖包，速度很慢。**只需设置一次，全局生效**。

```bash
# Windows / Linux / macOS 通用
go env -w GOPROXY=https://mirrors.aliyun.com/goproxy/,direct
go env -w GOSUMDB=off

# 查看是否设置成功
go env GOPROXY
# 预期输出：https://mirrors.aliyun.com/goproxy/,direct
```

**用户可以自己改**
```bash
go env -w GOPROXY=https://goproxy.cn,direct      # 换其他源
go env -u GOPROXY                                 # 恢复官方默认
go env GOPROXY                                    # 查看当前值
```

---

## 三、项目结构

```
server/
├── cmd/
│   ├── dhp-review/         # 评审 CLI 入口（给 GitHub Actions 用）
│   │   └── main.go
│   └── registry/           # HTTP Registry 服务入口（给企业部署用）
│       └── main.go
├── internal/
│   ├── ai/                 # AI 评审封装
│   ├── config/             # 配置文件解析
│   ├── rules/              # 8 条评审规则
│   └── storage/            # 文件存储（本地磁盘）
├── deploy/
│   ├── systemd/            # Linux systemd 服务文件
│   └── scripts/
│       └── install.sh      # 一键安装脚本
├── go.mod                  # Go 模块定义（类似 package.json）
└── config.yaml             # 运行时配置样例
```

---

## 四、编译

### 方法一：Windows 本地编译（开发调试）

在 `server/` 目录下打开终端：

```bash
cd /c/Project/Halo/digital-human-protocol/server

# 下载依赖
go mod tidy

# 编译评审 CLI
go build -o dhp-review.exe ./cmd/dhp-review

# 编译 Registry HTTP 服务
go build -o dhp-registry.exe ./cmd/registry
```

编译后目录下会多出两个 `.exe` 文件：
- `dhp-review.exe` — 命令行评审工具
- `dhp-registry.exe` — HTTP 服务

### 方法二：交叉编译 Linux 二进制（推荐，部署用）

你的开发机是 Windows，但服务器是 Linux，需要**交叉编译**：

```bash
cd /c/Project/Halo/digital-human-protocol/server

# 下载依赖
go mod tidy

# 交叉编译：生成 Linux 可执行文件（无 .exe 后缀）
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -o dhp-review ./cmd/dhp-review
go build -o dhp-registry ./cmd/registry
```

> **交叉编译的含义**：在 Windows 上运行编译命令，但输出的是 Linux 能运行的二进制文件。Go 原生支持，不需要额外工具。

---

## 五、打包

部署到 Linux 只需要这 **4 个文件**：

```
dhp-registry          # HTTP 服务二进制
config.yaml           # 配置文件
dhp-registry.service  # systemd 服务文件
install.sh            # 安装脚本（可选）
```

打包示例：
```bash
# Windows PowerShell
Compress-Archive -Path "dhp-registry","config.yaml","deploy\systemd\dhp-registry.service","deploy\scripts\install.sh" -DestinationPath "dhp-server.zip"
```

把 `dhp-server.zip` 传到 Linux 服务器上解压即可。

---

## 六、Linux 部署

### 方式 A：手动部署（理解原理）

假设你把二进制传到了服务器的 `/opt/dhp-registry/` 目录。

#### 1. 创建运行用户（安全隔离）

```bash
sudo useradd --system --home-dir /opt/dhp-registry --shell /bin/false dhp
```

#### 2. 准备目录和文件

```bash
sudo mkdir -p /opt/dhp-registry/halo-data
sudo cp dhp-registry config.yaml /opt/dhp-registry/
sudo chown -R dhp:dhp /opt/dhp-registry
```

#### 3. 修改配置

```bash
sudo nano /opt/dhp-registry/config.yaml
```

至少改这几项：
```yaml
auth:
  type: token
  token: "your-secret-token-here"   # <-- 改成你自己的强密码

listen: ":8080"                     # <-- 服务端口
```

#### 4. 创建 systemd 服务

把 `dhp-registry.service` 复制到系统目录：

```bash
sudo cp dhp-registry.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable dhp-registry
sudo systemctl start dhp-registry
```

#### 5. 验证运行

```bash
# 查看服务状态
sudo systemctl status dhp-registry

# 查看实时日志
sudo journalctl -u dhp-registry -f

# 测试接口
curl http://localhost:8080/skills.json
```

### 方式 B：一键脚本部署（快速）

把编译好的 `dhp-registry` 和 `config.yaml` 放到服务器上，然后：

```bash
sudo bash deploy/scripts/install.sh
```

脚本会自动完成：创建用户、复制文件、安装 systemd 服务、启动服务。

> **注意**：运行前请先编辑 `config.yaml` 里的 `token`，否则默认是 `REPLACE_ME`。

---

## 七、配置文件详解

```yaml
storage:
  type: local
  config:
    path: "./halo-data"           # 数据存储目录（相对工作目录）

ai_judge:
  endpoint: ""                    # AI 评审接口地址（空=跳过 AI 评审）
  model: ""
  api_key: ""                     # API Key

rules:
  enabled:                        # 启用的评审规则
    - schema_valid
    - slug_unique
    - dangerous_permissions
    - skill_code_safety
    - prompt_quality
    - metadata_compliance
    - duplicate_detection
    - sensitive_categories

auth:
  type: token
  token: "REPLACE_ME"             # <-- 必须修改！客户端上传时带 Bearer Token

auto_merge_threshold: "low_risk"  # 自动通过阈值
listen: ":8080"                   # 监听端口
```

---

## 八、常用命令速查

| 命令 | 作用 |
|---|---|
| `go version` | 查看 Go 版本 |
| `go env GOPROXY` | 查看当前镜像源 |
| `go mod tidy` | 下载/整理依赖 |
| `go build -o xxx ./cmd/xxx` | 编译 |
| `GOOS=linux GOARCH=amd64 go build ...` | 交叉编译 Linux 二进制 |
| `sudo systemctl start dhp-registry` | 启动服务 |
| `sudo systemctl stop dhp-registry` | 停止服务 |
| `sudo systemctl restart dhp-registry` | 重启服务 |
| `sudo systemctl status dhp-registry` | 查看状态 |
| `sudo journalctl -u dhp-registry -f` | 实时查看日志 |

---

## 九、常见问题

### Q1: 服务器没有 Go，能跑吗？
**能。** 编译后的 `dhp-registry` 是独立二进制，服务器上**不需要安装 Go**。

### Q2: 怎么更新到新版？
```bash
# 1. 重新编译
go build -o dhp-registry ./cmd/registry

# 2. 上传到服务器替换
sudo cp dhp-registry /opt/dhp-registry/
sudo systemctl restart dhp-registry
```

### Q3: 防火墙怎么开？
```bash
# CentOS/RHEL (firewalld)
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload

# Ubuntu/Debian (ufw)
sudo ufw allow 8080/tcp
```

### Q4: 数据目录在哪？
默认在 `/opt/dhp-registry/halo-data/` 下，按 `{slug}@{version}/` 存放。
