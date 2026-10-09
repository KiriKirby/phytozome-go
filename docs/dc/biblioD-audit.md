# Dr. Nelson biblioD audit

审计日期：2026-10-09。

`https://drnelson.uthsc.edu/wp-content/uploads/sites/130/resources/biblioD.html`
是 Dr. David Nelson 的历史植物 P450 文献/序列汇编页面，不是现代 API，也不是一个可直接镜像进 PGD 的规范化数据集。页面自述覆盖 1993 年命名法更新后出现的植物 CYP，页面最后修改于 2010-06-22，数据列表最后更新于 2008-03-24。

页面按 CYP51、CYP71–CYP99、CYP701–CYP719 等家族排列，混合了 accession、GenBank/GenEMBL/JGI 模型、文献、功能说明、完整蛋白、EST、短片段、alignment-only 片段、重复条目以及明确标注的错误记录。它还记录了命名历史和排除理由，例如 D64052 tobacco 条目实际更像 rRNA，部分“植物 CYP1”其实是植物中表达的 rat CYP1A1。

## 对 PGD 的可用价值

- 用于 provenance/accession 补充、历史版本对照、家族命名解释和审计注释。
- 可对照当前已审核的 Chlamydomonas、Volvox、Micromonas、Ostreococcus、Physcomitrella 等资源，发现历史 accession 或旧命名。
- 不能整体导入：页面明确包含重复、EST、短片段、对齐片段、错误/非 P450 序列，且缺少当前 PGD 的资源块关系。
- 任何新增序列都必须回到对应资源逐条人工核验；禁止用 biblioD 作为缺失 FASTA 的自动补全来源。

页面链接的 `72clan.pdf`、`CYP80.pdf`、`cyp92e.pdf` 是家族/命名论文附件，同样应作为历史 provenance 和命名参考，不应直接作为序列导入源。`rice.CYP72As.html` 当前返回 404。旧的 red algae 子页已迁移为 `redalgae.doc`，其内容已由数据库仓库的专属 parser 按原始 alignment 和子文档人工核验。

## 运行时策略

主程序只消费已发布、带 manifest SHA-256/size 校验的 PGD。biblioD 不参与运行时网络回退；这样可以避免历史错误、重复或非完整片段污染搜索结果。原始下载副本保留在仓库根目录 `biblioD.html` 作为审计证据。
