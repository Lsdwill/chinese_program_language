# 华言 v0.3.0

华言（Huayan）是一门使用中文关键字的独立脚本语言。仓库包含 UTF-8 Lexer、递归下降/Pratt Parser、Resolver、局部槽位/Upvalue 字节码、栈式 VM、闭包、集合、记录、异常、模块、中文标准库和模块化图书馆示例。

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

源文件使用 `.hua` 扩展名，也接受 `.华` 作为模块文件别名。语言实现位于 `internal/`，执行路径始终是：

```text
源文件 → Lexer → Parser → Resolver → 字节码 → VM
```

## 当前边界

已实现核心语言和常用标准模块；完整图书馆业务示例作为应用验证。尚未实现类、泛型、协程、包管理器、JIT 和数据库驱动，这些与开发计划中的第一版不做清单一致。

## 测试

```bash
go test ./...
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
- `docs/错误信息与故障排查.md`：错误类别和排查方法；
- `docs/常见问题.md`：安装、模块和数据问题；
- `docs/Python与Lua迁移.md`：迁移差异；
- `docs/兼容政策.md`：版本兼容边界。
