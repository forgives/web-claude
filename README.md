# web-claude

一个本地运行的 Go Web 项目，用浏览器封装本机已安装的 `claude` CLI，并尽量保持原生终端体验。本项目弥补了大家不能24小时在电脑前面加班的遗憾，让你可以随时随地的加班写代码写文档，使得你的人生更加充实，闪闪发光。

该项目完全由 AI 编写 。

## 当前实现

- Gin + `gorilla/websocket` + `creack/pty`
- `xterm.js` + fit addon 全屏终端渲染
- 前端终端资源已内置在本地静态目录，不依赖外部 CDN
- 登录认证基于本地配置中的 bcrypt 密码哈希和签名 cookie
- 登录 session 有效期为 7 天
- 默认仅监听 `127.0.0.1:8081`
- 默认纯黑终端界面，无侧边栏、无额外表单
- 浏览器断开后保留 `claude` 进程，重连继续附加
- 支持通过配置文件指定 `claude` 进程启动时的工作目录
- 前端输入经 WebSocket 转发到 PTY，PTY 输出实时回传到浏览器
- WebSocket 输入会过滤危险控制序列，限制异常 payload

## 运行

启动服务：

```bash
go run .
```

默认配置建议监听 `127.0.0.1:8081`，仅本机访问时打开 `http://127.0.0.1:8081`。

服务启动前需要保证本机 `PATH` 中可以找到 `claude` 命令。

如果你希望打开页面后直接在某个项目目录中运行 `claude`，可以在 `data/config.json` 中设置 `working_dir`，服务会按这个目录启动 `claude` 进程，而不是按当前启动命令所在目录启动。

首次使用前先设置登录密码：

```bash
go run . -set-password
```

设置完成后，密码哈希和 session secret 会写入 `data/config.json`。

## 配置

配置文件默认路径是 `data/config.json`。可选字段：

- `allow_remote_access`: 默认 `false`。如果要监听非本机地址，必须显式改为 `true`
- `listen_addr`: 可选，默认 `127.0.0.1:8081`
- `password_hash`: 登录密码的 bcrypt 哈希
- `restart_on_reconnect`: 默认 `false`。为 `true` 时，用户重连会重启终端进程而不是附加到旧会话
- `working_dir`: 可选，`claude` 进程启动时使用的工作目录；未配置时默认使用服务启动目录
- `session_secret`: 用于签名认证 cookie 的随机密钥
- `created_at`: 配置文件首次写入时间，UTC 时间
- `updated_at`: 配置最后一次更新的时间，UTC 时间

示例：

```json
{
  "allow_remote_access": true,
  "listen_addr": "0.0.0.0:8081",
  "password_hash": "$2a$10$exampleexampleexampleexampleexampleexampleexampleexample",
  "restart_on_reconnect": false,
  "working_dir": ".",
  "session_secret": "replace-with-random-secret"
}
```

### `data/config.json` 字段详解

#### `allow_remote_access`

- 作用：控制服务是否允许监听非本机地址。
- 默认行为：未开启时，只允许监听 `127.0.0.1`、`localhost` 或其他 loopback 地址。
- 典型用途：
  - 本机自用时保持 `false`
  - 需要局域网访问时改为 `true`
- 影响：
  - 当该值为 `false` 时，即使你把 `listen_addr` 改成 `0.0.0.0:8081`，服务也会因为安全校验而拒绝启动。
  - 当该值为 `true` 时，服务才允许绑定到局域网地址或所有网卡地址。

#### `listen_addr`

- 作用：指定 Web 服务实际监听的地址和端口。
- 格式：`主机:端口`，例如 `127.0.0.1:8081`、`0.0.0.0:8081`。
- 常见取值：
  - `127.0.0.1:8081`
    - 仅当前机器可访问。
    - 适合纯本机使用。
  - `0.0.0.0:8081`
    - 监听所有网络接口。
    - 适合局域网访问。
  - `192.168.1.23:8081`
    - 仅监听某一个指定的局域网 IP。
- 注意：
  - 如果这里配置的是非本机地址，`allow_remote_access` 必须同时为 `true`。
  - 修改后需要重启服务才会生效。

#### `password_hash`

- 作用：保存 Web 登录密码的 bcrypt 哈希值，不保存明文密码。
- 写入方式：
  - 推荐使用 `go run . -set-password` 自动生成。
  - 不建议手工编辑。
- 影响：
  - 登录页提交的密码会和这里的哈希进行比对。
  - 如果该字段为空，服务会认为认证未配置完成，并拒绝正常启动。
- 说明：
  - 每次重新设置密码后，这个值都会变化，即使新旧密码相同，bcrypt 生成的哈希也可能不同。

#### `restart_on_reconnect`

- 作用：控制浏览器断开后再次连接时，后端如何处理已有的 `claude` 进程。
- 可选行为：
  - `false`
    - 默认值。
    - 重连时会尝试附加到已有终端会话。
    - 适合持续对话和长任务执行。
  - `true`
    - 每次重连都会终止旧会话并重新启动一个新的 `claude` 进程。
    - 适合你希望每次打开页面都拿到干净终端的场景。
- 影响：
  - 当值为 `false` 时，浏览器刷新后通常还能看到已有会话内容。
  - 当值为 `true` 时，重连后上下文会丢失，因为终端进程会重新创建。

#### `working_dir`

- 作用：指定后端启动 `claude` CLI 时使用的工作目录。
- 默认行为：
  - 未配置时，继续使用 `go run .` 或编译后二进制启动时所在的当前目录。
  - 配置后，新的 `claude` 进程会在该目录下启动，而不是固定使用服务启动目录。
- 支持格式：
  - 绝对路径，例如 `/Users/name/workspace/my-project`
  - 相对路径，例如 `.`、`..`、`../another-project`
- 相对路径解析规则：
  - 相对于服务启动目录解析，不是相对于 `data/config.json` 所在目录。
- 注意：
  - 路径必须真实存在，且必须是目录，否则服务会拒绝启动。
  - 修改后需要重启服务才会生效。

#### `session_secret`

- 作用：用于签名登录 session cookie，防止客户端伪造认证状态。
- 写入方式：
  - 推荐使用 `go run . -set-password` 自动生成。
  - 不建议手工编辑，且应保持足够随机。
- 影响：
  - 登录成功后浏览器拿到的 cookie 会基于该值签名。
  - 如果修改了该值，所有已有登录状态会立即失效，用户需要重新登录。
  - 如果该字段为空，服务会认为认证未配置完成，并拒绝正常启动。

#### `created_at`

- 作用：记录该配置文件第一次由程序写入的时间。
- 格式：UTC 时间，例如 `2026-03-24T02:13:28.527822Z`。
- 用途：
  - 仅用于记录配置创建时间。
  - 不参与业务逻辑判断。

#### `updated_at`

- 作用：记录配置最近一次被程序更新的时间。
- 格式：UTC 时间，例如 `2026-03-24T07:18:48.431405Z`。
- 典型变化场景：
  - 使用 `go run . -set-password` 重设密码
  - 后续如果程序增加更多配置写入逻辑，这个字段也会同步更新
- 用途：
  - 仅用于追踪配置变更时间。
  - 不参与认证或监听逻辑判断。

### 局域网访问

如果要让同一局域网内的其他设备访问，需要同时满足这两个条件：

- `allow_remote_access` 设为 `true`
- `listen_addr` 设为 `0.0.0.0:8081` 或指定当前机器的局域网 IP

示例：

```json
{
  "allow_remote_access": true,
  "listen_addr": "0.0.0.0:8081",
  "password_hash": "...",
  "restart_on_reconnect": false,
  "working_dir": ".",
  "session_secret": "..."
}
```

然后重启服务，并使用当前机器的局域网 IP 访问，例如：

```text
http://192.168.1.23:8081
```

如果仍然无法访问，通常需要继续检查：

- 系统防火墙是否拦截了 `8081`
- 路由器或公司网络是否禁止设备间互访
- 访问地址是否误用了 `127.0.0.1` 或 `localhost`
- 服务是否确实已经启动并监听在 `0.0.0.0:8081`

## 环境变量

- `WEB_CLAUDE_ADDR`: 覆盖监听地址；如果是外网地址，仍要求配置文件里 `allow_remote_access=true`
- `WEB_CLAUDE_DATA_DIR`: 覆盖 `data` 目录位置

## 项目结构

目录说明如下：

- `data/`: 本地运行配置目录，默认存放 `config.json`
- `internal/`: 后端内部实现目录，不直接对外暴露
- `internal/auth/`: 认证与输入处理，包括密码哈希、session 签名、终端输入过滤
- `internal/config/`: 配置加载、配置保存、监听地址校验
- `internal/terminal/`: PTY 会话管理、进程重连附加、历史缓冲和终端尺寸同步
- `static/`: Web 前端静态资源目录
- `static/vendor/`: 前端第三方依赖的本地副本目录
- `static/vendor/xterm/`: `xterm.js` 运行时脚本、样式和许可证文件
- `static/vendor/xterm-addon-fit/`: `xterm-addon-fit` 脚本和许可证文件

主要文件说明：

- `main.go`: 服务启动入口、静态资源加载、认证路由、WebSocket 路由
- `requirement.md`: 原始需求文档
- `README.md`: 项目说明、配置说明和运行方式
- `go.mod`: Go 模块定义和直接依赖
- `go.sum`: Go 依赖校验信息

## 说明

- 服务直接调用本机 `claude` 命令，不提供任何 API Key 配置表单
- 当前会话状态保存在进程内存中；配置仍然走本地文件
- 登录成功后浏览器会收到一个 HttpOnly cookie，默认 7 天后失效
- 对于全屏 TUI，断线重连后的“历史回放”只能尽量恢复，不等价于真正的终端 screen buffer
