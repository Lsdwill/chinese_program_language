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

源文件使用 `.hua` 扩展名，也接受 `.华` 作为模块文件别名。语言实现位于 `internal/`，执行路径始终是：

```text
源文件 → Lexer → Parser → Resolver → 字节码 → VM
```

## 当前边界

已实现核心语言、常用标准模块、能力权限上下文、资源管理、中文数据库 API 和查询语法糖；完整图书馆业务示例作为应用验证。尚未实现类、泛型、协程、包管理器、LSP、HTTP 服务和 JIT。

## 测试

```bash
go test ./...
go test -race ./...
go vet ./...
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

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
