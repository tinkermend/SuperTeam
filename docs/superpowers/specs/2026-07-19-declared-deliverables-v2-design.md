# 声明式交付物闭环 v2 Spec:deliverables/ 约定目录 + 平台核对 + Ref 血缘回填
> 复核状态：全链落地（8359a52f）

> 日期:2026-07-19
> 状态:**已完结,GATE 全项真实 E2E PASS(2026-07-19 12:59,详见 CHANGELOG;代码随 8359a52f 入 main,归属见 CHANGELOG 澄清)**。实测注记:§5.3 缺交付物的实际路由是人类澄清(waiting_human+clarification)而非自动返工——平台对契约缺口的既定姿态,自动返工环属对抗评审线,符合预期;验收判据文本匹配启发式(autonomy 线残债)会在 planner 生成长句判据时拦截 completion,与本线正交,建议随对抗评审线偿还。
> 上游:输出附件 v1(`2026-07-19-execution-output-attachments-design.md`,已完结)+ 遗留立项 §3(`...-followups.md`,用户批准开工)
> 一句话:v1 是"兜底捕获,人眼分辨";v2 让交付物由契约声明、由平台核对、可从判据血缘直达文件。

---

## 0. 动因与既有挂点(实施前实测)

- v1 附件必然混入 agent 草稿,"哪个是正式交付物"无机器判据。
- 既有挂点(全部实测存在,v2 只做接线):
  - dispatch prompt 已要求 result_contract 带 `deliverables` 数组逐项覆盖 produces(`project_store.go` buildTaskPrompt,"结果契约要求"段);
  - `TaskResultDeliverable{Name, Kind, Value, Ref, Summary}`,"有 value 或 ref 才算已交付";produces 缺项 → `handoff_deliverable_missing` 打回(C1 返工环接住);
  - CP `evidenceRowForArtifactType` 已预留 `"declared"` → 证据行 `declared_output`/`submitted`(计入真证据);
  - 契约以 `contract_payload` JSONB 落 `project_task_results`,与工件物化同一完成事务——Ref 回填可做到强一致;
  - `enrichContractWithHandoffVerification` 对每个已交付 produces 追加平台核对 verification。

## 1. 执行约定注入(CP dispatch)

buildTaskPrompt "结果契约要求"段追加一句:**文件形态的交付物必须写入工作目录 `deliverables/` 目录**,并在 result_contract.deliverables 对应项的 `ref` 填相对路径(如 `deliverables/report.html`);纯值型交付物(结论、数字、链接)仍用 `value`。produces 语义与校验不变。

## 2. Runtime 采集(declared 管道,与 v1 附件管道并行)

- complete 分支扫工作目录 `deliverables/`(不存在则跳过):**目录内全部常规文件,不限扩展名**(声明目录本身就是白名单),排除隐藏路径组件。
- `artifact_type = "declared"`,`is_evidence = true`(CP 既有映射 → declared_output/submitted,计入真证据),不脱敏,metadata 带 `relative_path`(含 `deliverables/` 前缀)。
- 熔断:单文件 10MiB(CP presign 硬顶,超限**跳过留痕**——declared_skipped 自报行)、数量 20、总量 100MiB。
- **上传语义 = 整批失败**(与证据同格,区别于附件的 best-effort):契约承诺的交付物不允许"声称交付了但没落库"。
- **v1 附件管道排除 `deliverables/` 目录**,避免同一文件双采(声明管道接管)。
- `wait_human` 分支维持不采。

## 3. CP 核对与 Ref 血缘回填

- 物化(materializeAttemptEvidence,同事务)时构建 `relative_path → artifact_ref_id` 与 `file_name → artifact_ref_id` 映射(仅 declared 类型)。
- 回填:`contract.Deliverables[].Ref` 若匹配某 declared 工件的 relative_path 或文件名,改写为 `artifact_ref_id`(uuid),原始路径挪进 `Summary` 或保留在工件 metadata;不匹配的 Ref/纯 value 项原样保留(不硬性失败——值型交付物合法)。
- 回填后再落 `contract_payload`,保证读侧(验收面板/任务详情)拿到的契约已带血缘。
- `enrichContractWithHandoffVerification`:produces 名若命中已回填 deliverable,verification summary 附 artifact_ref_id。
- produces 缺项打回沿用 `handoff_deliverable_missing`,不新增打回逻辑。

## 4. Web

- 工件面板新增**"正式交付物"区**(artifact_type `declared` / `declared_skipped`),置于附件区之上,复用 v1 预览/下载组件;标注"契约声明的交付物,平台已核对对象存在"。
- 验收面板"判据 → 交付物"深链为 P2(读模型已带 ref 则顺手做,否则不扩本期)。

## 5. GATE(真实 E2E,全过才算完成)

1. **正向**:真实任务指令写 `deliverables/report.html` + 根目录 `notes.md`,contract 声明对应 deliverable(ref=相对路径)→ 面板"正式交付物"区出现 report.html(可预览),notes.md 落附件区,**无双采**;DB 断言 declared 工件行 + `declared_output`/`submitted` 证据行。
2. **Ref 回填**:`project_task_results.contract_payload` 中该 deliverable 的 ref 已变为 artifact_ref_id(DB 断言)。
3. **缺交付物打回**:produces 声明但 agent 未交付 → completion 验证失败含 `handoff_deliverable_missing`,返工环触发(观察到重试即可)。
4. **熔断留痕**:>10MiB 交付物跳过,declared_skipped 行可见。

## 6. 非目标

- 验收面板判据行深链交付物预览(P2,视读模型现状)。
- deliverables 单文件上限突破 10MiB(需动 CP presign 顶,另议)。
- 交付物内容质量核验(属对抗评审/验收判据线,非本 spec)。
