# 构建规划资源的完整提交与失败收敛

> **Workspace:** F:\swe\098905\project-02

---

**User**

一次构建在写入 build 之后，如果创建 service、step 或 build executable 时数据库报错，用户当前可能看到 error 或 pending 的半成品，随后重试又会用同一个 build number 冲突，甚至留下能够被误取的孤立资源。请让 webhook、schedule、手动创建和 restart 共用的规划流程具备完整提交语义：build、全部 service、全部 step 与 executable 要么作为一套可消费结果出现，要么进入可确定恢复且不会被正常读取或领取的状态；任何中途失败都不能让下一次重试因旧记录冲突，也不能生成第二套子资源。若清理或回滚本身只完成一部分，后续调用应能依据持久化状态继续收敛，而不是依赖本次进程内已经返回的对象列表。重试成功后只保留一个 build number、一套子资源和一个 executable；编译或路由校验发生在规划前的既有错误、需要审批的 pending build 以及正常成功路径保持原有可观察行为。

**Assistant**

I'll start by exploring the project structure to understand the codebase.

toolName: view_folder

status: failed

f:\swe\098905\project-02

toolName: file_search

status: success

toolName: Shell

status: success

This is a Go project that appears to be a CI/build system. Let me explore the key directories to understand the build planning flow.

toolName: todo_write

status: success

Todos updated: 6 items

toolName: Shell

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\go.mod

This is go-vela (Vela CI). Let me examine the key files for build creation flow.

toolName: Shell

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\build\create.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\build\plan.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\build\clean.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\build\compile_publish.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\build\executable.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\build\restart.go

Now let me look at the service/step planning functions and database layer.

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\build\gatekeep.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\service\plan.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\step\plan.go

Now let me examine the database layer interface and build-related DB operations.

toolName: Shell

status: success

toolName: file_search

status: success

toolName: file_search

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\build\create.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\build\delete.go

Let me examine the database types and the database interface.

toolName: file_search

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\interface.go

toolName: file_search

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\build\interface.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\types\build.go

toolName: file_search

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\build\enqueue.go

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\executable\create.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\executable\pop.go

Now let me look at the webhook handler and schedule execution flow.

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\webhook\post.go

Let me find all callers of CompileAndPublish and PlanBuild, plus the schedule processing code.

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\cmd\vela-server\schedule.go

Now let me examine the database engine structure to understand transaction capabilities.

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\database.go

Let me examine the database service/step create functions and their types.

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\resource.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\service\create.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\step\create.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\log\create.go

Let me check the mock layer to understand what needs updating when the interface changes.

toolName: Shell

status: success

toolName: search_by_regex

status: success

Let me look at how the netrc token is generated and other relevant pieces.

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\scm\github\repo.go

Let me check how step/service Environment is populated.

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\cache\redis\store_install.go

Now let me look at the database types for service, step, log, and executable.

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\types\service.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\types\log.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\types\build_executable.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\build\build.go

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

I have a thorough understanding now. Let me check a few remaining details: the step types, the integration test setup, and the build create test pattern.

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\integration_test.go

toolName: Shell

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\build\create_test.go

toolName: search_by_regex

status: success

toolName: todo_write

status: success

Todos updated: 8 items

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\types\step.go

The design is clear. Let me check how `NewTest` is used in existing database-package tests and the repo creation requirements.

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\database_test.go

toolName: search_by_regex

status: success

Now I'll implement. Starting with the atomic transaction method in the database layer:

toolName: todo_write

status: success

Todos updated: 8 items

toolName: Write

status: success

filePath: f:\swe\098905\project-02\database\plan.go

changes: undefined

Now add the method to the database Interface:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-02\database\interface.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-02\database\interface.go

changes: undefined

Now refactor the service planner into a pure in-memory builder:

toolName: Write

status: success

filePath: f:\swe\098905\project-02\api\service\plan.go

changes: undefined

toolName: search_by_regex

status: success

toolName: Write

status: success

filePath: f:\swe\098905\project-02\api\step\plan.go

changes: undefined

Now rewrite the API-layer PlanBuild to use the atomic transaction:

toolName: Write

status: success

filePath: f:\swe\098905\project-02\api\build\plan.go

changes: undefined

Now update `compile_publish.go` — move route validation before planning, call the new atomic PlanBuild, and remove the post-plan token/executable tail:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-02\api\build\compile_publish.go

changes: undefined

Now remove the now-redundant token/route/executable tail after the loop:

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\build\compile_publish.go

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-02\api\build\compile_publish.go

changes: undefined

Now remove the obsolete `PublishBuildExecutable` from executable.go:

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-02\api\build\executable.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-02\api\build\executable.go

changes: undefined

Now let me build to catch compile errors:

toolName: Shell

status: success

toolName: edit_file_search_replace

status: success

filePath: f:\swe\098905\project-02\database\plan.go

changes: undefined

toolName: Shell

status: success

Build passes. Let me verify the approve flow and other consumers remain compatible:

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\build\approve.go

Approve flow is compatible. Now run vet and the relevant existing tests:

toolName: Shell

status: success

Now let me write tests for the atomic planning. First, check the repo/user creation requirements:

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

Now create the test for the atomic planning transaction:

toolName: Write

status: success

filePath: f:\swe\098905\project-02\database\plan_test.go

changes: undefined

toolName: Shell

status: success

All new tests pass. Now run the full test suites for affected packages:

toolName: Shell

status: success

toolName: Shell

status: success

toolName: todo_write

status: success

Todos updated: 8 items

toolName: Shell

status: success

The `scm/github` failures look unrelated (I didn't touch that package). Let me verify they're pre-existing by checking what they do:

toolName: Shell

status: success

That failure is a pre-existing CRLF-vs-LF fixture issue on Windows (`13 10` vs `10`) in `scm/github`, which I never touched. Let me verify the remaining packages (api, cmd, compiler) pass:

toolName: Shell

status: success

toolName: Shell

status: success

The failing packages are ones I never touched. Let me confirm these failures are pre-existing by checking the test and verifying against a clean tree:

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\compiler\types\pipeline\ruleset_test.go

This is a dependency-version-sensitive regex test, unrelated to my change. Let me definitively confirm all three failing packages fail on the clean tree too:

toolName: Shell

status: success

All three failing packages fail identically on the pristine tree — pre-existing Windows/CRLF and dependency-version issues, unrelated to my work. Let me do a final review of the changed `compile_publish.go` flow:

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\build\compile_publish.go

Let me verify `queue.Route` is a pure function (safe to call before planning):

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\queue\redis\route.go

Pure function confirmed. Let me run gofmt and a final build:

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

Existing files are identically CRLF on this Windows checkout (git stores LF) — my files match the local convention; no action needed. Let me do a final review of the complete diff:

toolName: Shell

status: success

toolName: search_by_regex

status: success

The primitive DB methods remain in use by tests — correctly retained. Let me run vet across the whole module and the build-package tests once more:

toolName: Shell

status: success

Let me check existing test fakes in the build API package to see if an end-to-end wrapper test is cheap to add:

toolName: Shell

status: success

Let me verify that `Step.Environment()` / `Service.Environment()` don't depend on the DB-assigned ID (since I moved env merging before insert):

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\types\step.go

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\api\types\service.go

No ID dependency — env merging is identical pre-insert. Let me do a final read of the new database method to confirm quality:

toolName: view_files

status: success

filePath: f:\swe\098905\project-02\database\plan.go

All checks pass. Let me finalize the todo list:

toolName: todo_write

status: success

Todos updated: 8 items

已完成。核心思路是把"规划阶段"从多条独立写入改成**单数据库事务的原子提交**，并消除对进程内对象列表的依赖。

## 问题根因

webhook、schedule、手动创建、restart 共用 [CompileAndPublish](file:///f:/swe/098905/project-02/api/build/compile_publish.go)，原先的顺序是各自独立提交：`CreateBuild`（含 repo counter+1）→ 逐个 `CreateService`/日志 → 逐个 `CreateStep`/日志 → 取 token → 路由校验 → `CreateBuildExecutable`。中途失败时：

- [CleanBuild](file:///f:/swe/098905/project-02/api/build/clean.go) 只把 build 标成 error，并依赖内存里带回的列表去 kill 子资源（而 [PlanServices](file:///f:/swe/098905/project-02/api/service/plan.go) 实际从不 append，service 永远清不掉；日志和 executable 也完全不清）；
- 重试循环复用同一个 `b`（保留已分配的 ID/number），再调 CreateBuild 必然撞 `(repo_id, number)` 约束；
- 留下用户可见的 pending/error 半成品、孤立子资源和会被计数/误取的 executable。

## 改动

**1. 新增原子规划事务** [database/plan.go](file:///f:/swe/098905/project-02/database/plan.go)（挂到 [database.Interface](file:///f:/swe/098905/project-02/database/interface.go#L45-L49)）：在一个 gorm 事务内依次完成 counter+1 取号 → 插入 build → 插入全部 service 及占位日志 → 插入全部 step 及占位日志 → 回调内生成 SCM token 并序列化 executable → 压缩加密插入 executable。任何一步失败整体回滚：counter 不增、不留任何行；崩溃时未提交事务也会自动消失，恢复只取决于数据库的持久化状态而非内存列表。失败时还会把内存中已分配的 build ID/number 和子资源 ID 清零，保证重试干净。

**2. service/step 规划改为纯内存构造**：[api/service/plan.go](file:///f:/swe/098905/project-02/api/service/plan.go)、[api/step/plan.go](file:///f:/swe/098905/project-02/api/step/plan.go) 只负责建对象和 MergeEnv（已确认 Environment() 不依赖数据库生成的 ID），不再各自写库；report_as 的外部 SCM 状态上报移到事务提交后执行（[ReportStepStatuses](file:///f:/swe/098905/project-02/api/step/plan.go#L79-L115)），因为它不可回滚。

**3. 重写编排层**：[api/build/plan.go](file:///f:/swe/098905/project-02/api/build/plan.go) 通过事务方法一次提交全部资源；[compile_publish.go](file:///f:/swe/098905/project-02/api/build/compile_publish.go#L390-L428) 把路由校验移到规划之前（失败仍返回 400，但不再留下 error build），token 获取与 executable 落库进入同一事务，重试分支显式重置 ID/number。删除了不再需要的 `PublishBuildExecutable`（[executable.go](file:///f:/swe/098905/project-02/api/build/executable.go)）。

## 行为保持

- 编译失败、skip、部署校验等规划前错误：路径不变；规划失败现在与它们一样不留 build；
- 需要审批的 fork PR：Gatekeep 提交流程不变，executable 仍在待审批期间保留；
- 正常成功：201、队列项、SCM 状态、enqueue 等可观察行为不变；队列推送失败后的 CleanBuild（提交之后的失败）维持原样。

## 验证

新增 [database/plan_test.go](file:///f:/swe/098905/project-02/database/plan_test.go)：成功路径断言 1 个 build、一套 service/step、每个资源一条日志、executable 可 pop 且 counter=1；finalizer 失败/子资源非法时断言零行、counter 不增、内存 ID 清零，并验证随后重试收敛为**唯一**的 build number 1、唯一一套子资源和一个 executable。`go build`、`go vet`、database 与 api 全部相关包测试通过（`scm/github`、`compiler/...` 有 3 个失败，已用 stash 在未改动的干净树上复现，属 Windows CRLF/依赖版本的既有环境问题）。
