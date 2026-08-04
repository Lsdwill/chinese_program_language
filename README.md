# 华言 v0.4.0 开发中

华言（Huayan）是一门使用中文关键字的独立脚本语言。仓库包含 UTF-8 Lexer、递归下降/Pratt Parser、Resolver、局部槽位/Upvalue 字节码、栈式 VM、闭包、集合、记录、异常、模块、中文标准库、中文数据库查询能力和模块化图书馆示例。

## 运行

需要 Go 1.22 或更高版本：

```bash
go run ./cmd/huayan --version
go run ./cmd/huayan examples/你好世界.hua
go run ./cmd/huayan check examples/斐波那契.hua
go run ./cmd/huayan dis examples/斐波那契.hua
go run ./cmd/huayan test tests
go run ./cmd/huayan fmt --check examples/核心演示.hua
```

也可以构建独立解释器：

```bash
go build -o huayan ./cmd/huayan
./huayan examples/你好世界.hua
```

命令行入口：

| 命令 | 作用 |
|---|---|
| `huayan 文件.hua` | 执行华言源文件 |
| `huayan` | 进入交互式 REPL |
| `huayan -c '代码'` | 执行一段命令行代码 |
| `huayan check 文件.hua` | 只进行词法、语法和编译检查 |
| `huayan dis 文件.hua` | 查看生成的字节码 |
| `huayan fmt 文件.hua` | 输出格式化后的源码 |
| `huayan fmt --check 文件.hua` | 检查源码是否符合格式 |
| `huayan test 目录` | 递归执行 `.hua` 测试文件 |
| `huayan --version` | 查看解释器版本 |

命令也提供部分中文别名，例如 `检查`、`字节码`、`格式化` 和 `测试`。

## 最小示例

```text
函数 阶乘(数字)
    如果 数字 <= 1
        返回 1
    结束
    返回 数字 * 阶乘(数字 - 1)
结束

打印(阶乘(5))
```

## 中文数据库

华言提供中文数据库模块，不要求用户编写 SQL。底层数据库引擎属于运行时实现细节，源码使用中文 API 和查询语法：

```text
导入 标准.数据库 为 数据库

让 库 = 数据库.打开(":memory:")
数据库.建表(库, "图书", 记录 {编号: "文字", 书名: "文字"})
数据库.插入(库, "图书", 记录 {编号: "B001", 书名: "活着"})

让 结果 = 选择 图书 从 库
    其中 编号 等于 "B001"
    排序 书名 升序
    限制 20
结束

打印(结果[0].书名)
数据库.关闭(库)
```

数据库模块还支持 `选择一行`、`更新`、`删除`、`开始事务`、`提交` 和 `回滚`。查询表名、字段名和动态值都会经过安全校验或参数绑定。

数据库字段类型使用中文名称：`文字`、`整数`、`小数`、`布尔` 和 `字节串`。数据库操作需要“数据库”能力，数据库和事务属于 VM 资源，关闭后不能继续使用。

## 语言能力

当前语言支持：

- 中文关键字、中文标识符、全角标点和 Unicode NFC 规范化；
- 整数、小数、布尔、空值、文字、不可变字节串；
- 列表、字典、记录、函数、递归和闭包；
- 变量、常量、局部作用域、上值和模块导入；
- `如果`、`否则`、`当`、`遍历`、`跳出`、`继续`；
- `尝试`、`捕获`、`抛出`、`最后`；
- 列表索引、记录字段、函数调用和运算符表达式；
- 原生模块注册表、资源管理器和能力权限上下文；
- 标准控制台、文字、列表、字典、文件、编码、JSON、时间、数学、程序、测试和数据库模块。

示例：

```text
导入 标准.编码 为 编码

让 数据 = 编码.UTF8编码("华言")
打印(编码.十六进制编码(数据))
打印(编码.UTF8解码(数据))
```

## 图书馆示例

图书馆示例位于 `examples/图书馆/`，包含：

- 图书、读者和借阅记录模型；
- JSON 文件仓库；
- 图书、读者、借阅业务服务；
- 命令行菜单；
- 模块化入口和测试示例。

运行图书馆程序：

```bash
go build -o huayan ./cmd/huayan
./huayan examples/图书馆/主.hua
```

自动化验证：

```bash
scripts/test-library.sh ./huayan
```

源文件使用 `.hua` 扩展名，也接受 `.华` 作为模块文件别名。语言实现位于 `internal/`，执行路径始终是：

```text
源文件 → Lexer → Parser → Resolver → 字节码 → VM
```

## 当前边界

已实现核心语言、常用标准模块、能力权限上下文、资源管理、中文数据库 API 和查询语法糖；完整图书馆业务示例作为应用验证。尚未实现类、泛型、协程、包管理器、LSP、HTTP 服务和 JIT。

数据库查询当前第一版使用字段等值条件；复杂条件构造器、项目清单、依赖安装、SQLite 图书馆迁移和 HTTP 服务属于第二阶段后续任务。

## 代码结构

```text
cmd/huayan/       命令行入口和 REPL
internal/lexer/   UTF-8 词法分析器
internal/parser/  递归下降和 Pratt 表达式解析器
internal/ast/     抽象语法树及稳定 Dump
internal/compiler/名称解析和字节码编译器
internal/bytecode/字节码结构及验证器
internal/vm/      栈式虚拟机、标准库和资源运行时
internal/native/  原生模块能力注册表
internal/resource/资源生命周期管理
internal/capability/能力权限集合
internal/engine/  文件加载、模块解析和执行入口
internal/formatter/源码格式化器
tests/conformance/语言一致性测试
examples/图书馆/  完整应用示例
docs/             教程、规范、ADR 和 API 文档
```

## 执行架构

华言不是 Python 的翻译层，也不是 Go 上的语法包装，而是一条独立的解释执行链：

```text
华言源码
  → UTF-8 Lexer
  → Parser / AST
  → 名称解析与作用域分析
  → 华言字节码
  → 字节码验证
  → 栈式 VM
  → 中文标准库与原生资源
```

VM 对外只暴露华言 `Value`，文件、数据库和事务等宿主资源使用不透明资源句柄管理。原生模块通过能力集合检查文件、环境和数据库访问。

## 测试

```bash
go test ./...
go test -race ./...
go vet ./...
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

完整阶段验证：

```bash
go test ./...
go test -race ./...
go vet ./...
scripts/check-core-coverage.sh 85
go build -o /tmp/huayan ./cmd/huayan
scripts/check-conformance.sh /tmp/huayan
scripts/test-library.sh /tmp/huayan
```

Linux 发布包黑盒场景测试：

```bash
bash scripts/test-release-linux.sh dist/huayan-v0.4.0/huayan-v0.4.0-linux-amd64
```

该测试直接使用发布二进制，覆盖 Linux 文件读写、UTF-8 编码、中文数据库查询、事务回滚、命令行参数和运行时错误退出码。

当前基线验证包含 20 个一致性案例和图书馆端到端测试。

核心包覆盖率门禁：

```bash
scripts/check-core-coverage.sh 85
```

一致性案例位于 `tests/conformance/`，可直接运行：

```bash
go run ./cmd/huayan test tests/conformance
```

发布级一致性校验（比较标准输出、标准错误和退出码）：

```bash
go build -o huayan ./cmd/huayan
scripts/check-conformance.sh ./huayan
```

进一步阅读：

- `docs/教程.md`：入门、语言导览和图书馆示例；
- `docs/标准库API.md`：标准库接口；
- `docs/spec/SQLite中文查询DSL设计.md`：中文数据库查询设计；
- `docs/错误信息与故障排查.md`：错误类别和排查方法；
- `docs/常见问题.md`：安装、模块和数据问题；
- `docs/Python与Lua迁移.md`：迁移差异；
- `docs/兼容政策.md`：版本兼容边界。
- `docs/decisions/`：运行时、资源和权限设计决策；
- `华言第二阶段开发计划.md`：应用生态和工程化路线。

## 开发状态

当前仓库处于第二阶段开发中。`v0.3.0` 是已提交的基线版本，当前主分支继续加入能力上下文、中文数据库和查询语法糖。字节码格式只作为内部实现，不承诺跨版本加载兼容。
