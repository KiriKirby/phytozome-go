package prompt

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/KiriKirby/phytozome-go/internal/model"
	"github.com/KiriKirby/phytozome-go/internal/tui"
)

type Prompter struct {
	out                 io.Writer
	sessionPath         []string
	blastProgramPath    string
	rowStates           map[string]tui.RowSelectionState
	blastRunStates      map[string]tui.BlastRunSelectionState
	keywordSelection    []bool
	blastSelections     map[string][]bool
	blastRunSelected    map[string][][]bool
	blastFilterSettings model.BlastFilterSettings
	blastFilterFlags    map[string][]bool
	blastRunFilterFlags map[string][][]bool
}

var BlastFilterSuggest func(BlastFilterRequest) (BlastFilterSuggestion, error)

var invalidFileNameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)
var numericValuePattern = regexp.MustCompile(`[-+]?\d*\.?\d+(?:[eE][-+]?\d+)?`)

func columnHelp(en string, cn string, jp string) string {
	return "EN: " + strings.TrimSpace(en) + "\n中文：" + strings.TrimSpace(cn) + "\n日本語：" + strings.TrimSpace(jp)
}

func ColumnHelpEnglish(id string) string {
	help := ColumnHelpText(id)
	if strings.TrimSpace(help) == "" {
		return ""
	}
	const prefix = "EN:"
	if !strings.HasPrefix(help, prefix) {
		return strings.TrimSpace(help)
	}
	help = strings.TrimPrefix(help, prefix)
	if idx := strings.Index(help, "\n中文："); idx >= 0 {
		help = help[:idx]
	}
	return strings.TrimSpace(help)
}

var columnHelpText = map[string]string{
	"search_tern": columnHelp(
		"Original keyword, identifier, URL, or pasted term that produced this row. Use it to trace every result back to the exact input that generated it, especially when mixed protein IDs and report links are searched together.",
		"产生这一行结果的原始关键词、编号、URL 或粘贴内容。它用于把每个结果追踪回实际输入，尤其适合链接和 protein ID 混合搜索的时候。",
		"この行を生成した元のキーワード、ID、URL、または貼り付け入力です。リンクと protein ID を混ぜて検索した場合でも、各結果を実際の入力へ追跡できます。",
	),
	"label_name": columnHelp(
		"User-facing label assigned to this query or search term. It is used for grouping results, naming exported peptide records, keeping batch BLAST queries readable, and carrying meaningful gene/function names into downstream files.",
		"给当前查询或搜索词分配的用户标签名。它用于结果分组、导出肽序列记录命名、让批量 BLAST query 更容易阅读，并把有意义的基因或功能名称带入下游文件。",
		"この query または検索語に付けた表示用ラベルです。結果のグループ化、出力ペプチド名、バッチ BLAST query の可読性、下流ファイルへの遺伝子名・機能名の引き継ぎに使います。",
	),
	"source_database": columnHelp(
		"Original database that produced this row, such as Phytozome or lemna. External references like UniProt and InterPro add columns later, but this field tells you where the primary hit came from.",
		"产生这一行结果的原始数据库，例如 Phytozome 或 lemna。UniProt 和 InterPro 等外部参考库只是后续加列；这个字段说明主要命中来自哪里。",
		"この行を生成した元データベースです。Phytozome や lemna などを示します。UniProt や InterPro は後から列を追加する外部参照であり、この列は主要ヒットの由来を示します。",
	),
	"blast_program": columnHelp(
		"BLAST program used for this search, for example BLASTP, BLASTN, BLASTX, or TBLASTN. The program determines whether nucleotide or protein sequence space was compared and how coordinates should be interpreted.",
		"本次搜索使用的 BLAST 程序，例如 BLASTP、BLASTN、BLASTX 或 TBLASTN。它决定比较的是核酸还是蛋白序列空间，也影响坐标和阅读框的解释方式。",
		"この検索に使われた BLAST プログラムです。BLASTP、BLASTN、BLASTX、TBLASTN などがあります。核酸空間かタンパク質空間か、座標やフレームをどう読むかに関わります。",
	),
	"hit_number": columnHelp(
		"Rank of the BLAST hit for the current query as reported by the BLAST parser. Lower ranks are usually stronger according to the BLAST output order, but biological relevance should still be checked with identity, coverage, annotation, and external references.",
		"当前 query 的 BLAST 命中排名。排名越靠前通常越强，但生物学相关性仍需要结合 identity、coverage、注释以及外部参考信息一起判断。",
		"現在の query に対する BLAST ヒット順位です。通常は上位ほど強いヒットですが、生物学的妥当性は identity、coverage、注釈、外部参照も合わせて判断します。",
	),
	"hsp_number": columnHelp(
		"HSP segment number within a BLAST hit. A single subject can contain multiple high-scoring segment pairs; this value identifies which aligned segment the row represents.",
		"同一个 BLAST 命中内部的 HSP 片段编号。一个 subject 可能有多个 high-scoring segment pair；这个值说明这一行代表哪一个比对片段。",
		"BLAST ヒット内の HSP セグメント番号です。1 つの subject に複数の high-scoring segment pair がある場合、この行がどのアラインメント片段かを示します。",
	),
	"protein": columnHelp(
		"Target protein or sequence identifier from the original database. This is the main local identifier to open the source report, fetch peptide sequence, connect to gene/transcript metadata, and map to external references.",
		"原始数据库中的目标蛋白或序列编号。这是打开原始报告、获取肽序列、连接基因/转录本信息以及映射外部参考数据库时最主要的本地编号。",
		"元データベースにおけるターゲットタンパク質または配列 ID です。ソースレポートを開く、ペプチド配列を取得する、遺伝子/転写産物情報につなぐ、外部参照へマップする際の主要 ID です。",
	),
	"subject_id": columnHelp(
		"Subject identifier reported directly by BLAST. When it is empty or less readable, the program may display the protein identifier elsewhere, but this field preserves the BLAST-level subject name for debugging and traceability.",
		"BLAST 直接返回的 subject 编号。为空或不够可读时，程序可能在其他位置显示 protein 编号；但这个字段保留 BLAST 层面的 subject 名称，方便排查和追踪。",
		"BLAST が直接返した subject ID です。空または読みにくい場合は別の場所で protein ID を表示することがありますが、この列は BLAST レベルの subject 名を追跡用に保持します。",
	),
	"species": columnHelp(
		"Species or genome label for the target row. In cross-species work, this is the first field to check when comparing ortholog candidates, lineage-specific hits, or duplicated pathway genes.",
		"目标行所属的物种或基因组标签。做跨物种比较时，这是判断同源候选、谱系特异命中或通路基因复制情况时首先要看的字段。",
		"ターゲット行の種またはゲノムラベルです。種間比較では、オルソログ候補、系統特異的ヒット、経路遺伝子重複を確認する最初の項目です。",
	),
	"e_value": columnHelp(
		"BLAST expectation value. Smaller values indicate that the alignment is less likely to occur by chance in the searched database, but E-value should be interpreted together with coverage, target length, and conserved-domain evidence.",
		"BLAST 的期望值。数值越小表示该比对在数据库中随机出现的可能性越低，但需要结合 coverage、target length 和保守结构域证据一起解释。",
		"BLAST の期待値です。値が小さいほど、そのアラインメントが検索データベース内で偶然生じる可能性が低いことを示しますが、coverage、target length、保存ドメイン証拠と合わせて解釈します。",
	),
	"percent_identity": columnHelp(
		"Percentage of identical residues or bases inside the aligned region. It measures local similarity within the alignment span, not whole-protein identity, so a short high-identity match can still be biologically incomplete.",
		"比对区域内部完全相同的氨基酸或碱基百分比。它衡量的是比对跨度内的局部相似性，不等于全长蛋白 identity；短片段高 identity 仍可能是不完整命中。",
		"アラインメント領域内で一致したアミノ酸または塩基の割合です。全長タンパク質の identity ではなく局所的な類似度なので、短い高 identity ヒットでも不完全な場合があります。",
	),
	"align_query_length_percent": columnHelp(
		"Alignment length divided by query length, shown as a percentage. This is a quick query-coverage signal: high values mean most of the query participates in the hit, while low values often indicate a partial domain-only match.",
		"比对长度除以 query 长度后的百分比。这是快速判断 query coverage 的指标：高值说明 query 大部分参与比对，低值常提示只是局部结构域命中。",
		"アラインメント長を query 長で割った割合です。query coverage の簡易指標で、高い値は query の大部分がヒットしていること、低い値はドメインだけの部分ヒットである可能性を示します。",
	),
	"interpro_conserved_region_status": columnHelp(
		"Derived InterPro status for conserved-region support. With a query InterPro template, the program compares query conserved evidence against each hit; without a query template, it grades the hit's own conserved-domain evidence. present means enough conserved evidence is found, partial means related but incomplete evidence exists, missing means InterPro lookup succeeded but expected conserved evidence was not found, and uncertain means the evidence is too weak or ambiguous.",
		"由 InterPro 派生的保守区域支持状态。有 query InterPro 模板时，程序会把 query 的保守证据与每个 hit 比较；没有 query 模板时，会保守地评价 hit 自身的保守结构域证据。present 表示证据足够，partial 表示有相关但不完整的证据，missing 表示查询成功但没有找到预期保守证据，uncertain 表示证据太弱或不明确。",
		"InterPro から導いた保存領域サポート状態です。query の InterPro テンプレートがある場合は query の保存証拠を各 hit と比較し、ない場合は hit 自身の保存ドメイン証拠を評価します。present は十分な証拠、partial は関連するが不完全な証拠、missing は検索成功だが期待証拠なし、uncertain は証拠が弱いまたは曖昧な状態です。",
	),
	"interpro_entry_name": columnHelp(
		"InterPro entry names matched on the hit protein. These names summarize families, domains, repeats, sites, or superfamilies and help you see whether the hit carries annotations expected for the pathway protein being studied.",
		"命中蛋白匹配到的 InterPro 条目名称。这些名称概括家族、结构域、repeat、site 或超家族，可帮助判断 hit 是否带有研究通路蛋白应有的注释。",
		"hit タンパク質にマッチした InterPro エントリー名です。ファミリー、ドメイン、リピート、サイト、スーパーファミリーを要約し、研究対象の経路タンパク質に期待される注釈を持つか確認できます。",
	),
	"interpro_entry_type": columnHelp(
		"InterPro entry types matched on the hit, such as family, domain, homologous superfamily, repeat, or site. Type information helps separate broad family evidence from specific catalytic or binding-site evidence.",
		"命中蛋白匹配到的 InterPro 条目类型，例如 family、domain、homologous superfamily、repeat 或 site。类型信息能帮助区分宽泛的家族证据与更具体的催化位点或结合位点证据。",
		"hit にマッチした InterPro エントリータイプです。family、domain、homologous superfamily、repeat、site などがあります。広いファミリー証拠と特定の触媒/結合サイト証拠を区別できます。",
	),
	"interpro_coverage_percent": columnHelp(
		"Approximate percentage of the hit protein covered by InterPro match regions. Higher values suggest the hit has substantial annotated conserved content; lower values can indicate a small domain fragment or sparse annotation.",
		"命中蛋白被 InterPro 匹配区域覆盖的大致百分比。较高值说明 hit 有较多被注释的保守内容；较低值可能表示只有小结构域片段或注释较少。",
		"hit タンパク質が InterPro マッチ領域で覆われるおおよその割合です。高い値は保存注釈が多いこと、低い値は小さなドメイン断片または注釈が少ないことを示す場合があります。",
	),
	"interpro_match_regions": columnHelp(
		"InterPro and Pfam coordinate ranges on the hit protein. These regions show where conserved signatures sit on the sequence, which is useful for checking truncation, domain order, and whether a BLAST alignment overlaps the functional region.",
		"InterPro 和 Pfam 在命中蛋白上的坐标范围。这些区域说明保守 signature 位于序列的哪里，可用于检查截短、结构域顺序，以及 BLAST 比对是否覆盖功能区域。",
		"hit タンパク質上の InterPro/Pfam 座標範囲です。保存 signature が配列上のどこにあるかを示し、短縮、ドメイン順序、BLAST アラインメントが機能領域と重なるかの確認に役立ちます。",
	),
	"interpro_accessions": columnHelp(
		"All InterPro accessions matched on the hit protein. These IPR identifiers are stable handles for integrated entries and are useful for comparing annotations across species and database releases.",
		"命中蛋白匹配到的所有 InterPro accession。这些 IPR 编号是整合条目的稳定标识，适合用于跨物种和跨数据库版本比较注释。",
		"hit タンパク質にマッチした InterPro accession の一覧です。IPR ID は統合エントリーの安定した識別子であり、種間比較やデータベース更新間の注釈比較に役立ちます。",
	),
	"interpro_signature_accessions": columnHelp(
		"Underlying member-database signature accessions supporting the InterPro matches. They expose the raw evidence behind an integrated InterPro entry and can reveal which member database supplied the support.",
		"支持 InterPro 匹配的底层成员数据库 signature accession。它们展示整合 InterPro 条目背后的原始证据，也能看出是哪个成员数据库提供了支持。",
		"InterPro マッチを支える下位メンバーデータベース signature accession です。統合 InterPro エントリーの背後にある生の証拠と、どのメンバーデータベースが支えたかを示します。",
	),
	"interpro_pfam_accessions": columnHelp(
		"Pfam accessions reported through InterPro for this hit protein. Pfam IDs are often especially useful for conserved-domain checks because they represent curated protein family/domain models.",
		"InterPro 为该命中蛋白报告的 Pfam accession。Pfam ID 通常特别适合用于保守结构域检查，因为它们代表人工整理的蛋白家族/结构域模型。",
		"この hit タンパク質について InterPro から報告された Pfam accession です。Pfam ID は curated protein family/domain model を表すため、保存ドメイン確認に特に有用です。",
	),
	"target_length": columnHelp(
		"Target sequence length from the original database. Compare it with query length, alignment length, and UniProt canonical length to detect truncated proteins, unusually long isoforms, fragments, or annotation mismatches.",
		"原始数据库中的目标序列长度。可与 query length、alignment length 和 UniProt canonical length 对比，用来发现截短蛋白、异常长 isoform、片段或注释不匹配。",
		"元データベースのターゲット配列長です。query length、alignment length、UniProt canonical length と比較することで、短縮タンパク質、異常に長い isoform、断片、注釈不一致を検出できます。",
	),
	"target_uniprot_canonical_length_percent": columnHelp(
		"Original-database target_length divided by UniProt canonical protein length, shown as a percentage. Values near 100 suggest the source protein length is consistent with the UniProt canonical reference; much lower or higher values warn that the hit may be truncated, extended, or mapped to a different isoform.",
		"原始数据库的 target_length 除以 UniProt canonical protein length 后得到的百分比。接近 100 表示原始数据库蛋白长度与 UniProt 标准参考较一致；明显偏低或偏高提示 hit 可能截短、延长或映射到不同 isoform。",
		"元データベースの target_length を UniProt canonical protein length で割った割合です。100 に近い値は UniProt 標準参照と長さが一致することを示し、大きく低い/高い値は短縮、延長、別 isoform へのマッピングを示唆します。",
	),
	"align_len": columnHelp(
		"Length of the BLAST alignment. It is the span actually aligned between query and target, so it should be considered together with query length and target length rather than treated as a full-length match by itself.",
		"BLAST 比对区域的长度。它表示 query 与 target 实际参与比对的跨度，需要和 query length、target length 一起看，不能单独当作全长命中。",
		"BLAST アラインメントの長さです。query と target が実際に並んだ範囲なので、単独で全長ヒットとみなさず、query length と target length と合わせて見ます。",
	),
	"query_length": columnHelp(
		"Length of the query sequence used for BLAST. It is the denominator for query-coverage calculations and helps distinguish full-length protein searches from domain-only or fragment searches.",
		"用于 BLAST 的 query 序列长度。它是计算 query coverage 的分母，也能帮助区分全长蛋白搜索、结构域搜索或片段搜索。",
		"BLAST に使った query 配列の長さです。query coverage の分母であり、全長タンパク質検索、ドメインのみの検索、断片検索を区別する助けになります。",
	),
	"strands": columnHelp(
		"Query and target strand or translated frame direction reported by BLAST. This is especially important for nucleotide-based programs because orientation and frame affect how coordinates and gene models should be interpreted.",
		"BLAST 返回的 query 和 target 链方向或翻译阅读框方向。对核酸相关程序尤其重要，因为方向和 frame 会影响坐标和基因模型解释。",
		"BLAST が返す query と target の鎖方向または翻訳フレーム方向です。核酸系プログラムでは、向きと frame が座標や遺伝子モデルの解釈に影響するため特に重要です。",
	),
	"query_id": columnHelp(
		"Query sequence identifier reported by BLAST. In batch runs it helps connect a hit back to the exact FASTA entry, URL-derived sequence, or generated query record used in the search.",
		"BLAST 返回的 query 序列编号。在批量运行中，它帮助把 hit 追踪回具体 FASTA 条目、由 URL 得到的序列或程序生成的 query 记录。",
		"BLAST が返した query 配列 ID です。バッチ実行では、hit を特定の FASTA エントリー、URL 由来配列、生成された query レコードへ結び付けます。",
	),
	"query_from": columnHelp(
		"Start coordinate of the aligned region on the query sequence. Together with query_to, it shows which part of the query supports the hit and whether important termini or domains are missing.",
		"query 序列上比对区域的起始坐标。与 query_to 一起说明 query 的哪一段支持该 hit，以及是否缺失重要端部或结构域。",
		"query 配列上のアラインメント開始座標です。query_to と合わせて、query のどの部分が hit を支えているか、重要な末端やドメインが欠けていないかを示します。",
	),
	"query_to": columnHelp(
		"End coordinate of the aligned region on the query sequence. Use it with query_from and query_length to see whether the hit covers the catalytic region, a single domain, or nearly the full query.",
		"query 序列上比对区域的结束坐标。可与 query_from 和 query_length 一起判断 hit 覆盖的是催化区域、单个结构域还是接近全长 query。",
		"query 配列上のアラインメント終了座標です。query_from と query_length と合わせて、hit が触媒領域、単一ドメイン、ほぼ全長 query のどれを覆うか判断できます。",
	),
	"target_from": columnHelp(
		"Start coordinate of the aligned region on the target sequence. This helps locate the hit inside the target protein and compare BLAST alignment position with InterPro match regions.",
		"target 序列上比对区域的起始坐标。它帮助定位 hit 位于目标蛋白的哪一段，并可与 InterPro 匹配区域比较。",
		"target 配列上のアラインメント開始座標です。hit がターゲットタンパク質のどこに位置するかを示し、InterPro マッチ領域との比較に使えます。",
	),
	"target_to": columnHelp(
		"End coordinate of the aligned region on the target sequence. Use it with target_from and InterPro match regions to check whether the BLAST hit overlaps conserved or functional regions.",
		"target 序列上比对区域的结束坐标。可与 target_from 和 InterPro 匹配区域一起检查 BLAST 命中是否覆盖保守或功能区域。",
		"target 配列上のアラインメント終了座標です。target_from と InterPro マッチ領域と合わせて、BLAST hit が保存領域や機能領域と重なるか確認できます。",
	),
	"bitscore": columnHelp(
		"BLAST bit score. Higher values indicate stronger alignments independent of database size, making it useful for ranking hits within the same query and comparing HSP strength.",
		"BLAST bit score。数值越高表示比对越强，并且相对不受数据库大小影响，适合在同一 query 内排序 hit 和比较 HSP 强度。",
		"BLAST bit score です。値が高いほど強いアラインメントを示し、データベースサイズに依存しにくいため、同一 query 内の hit 順位や HSP 強度比較に役立ちます。",
	),
	"mismatches": columnHelp(
		"Number of mismatched positions in the alignment. It helps explain why percent identity is lower and can highlight divergent regions even when the total alignment length is long.",
		"比对中的不匹配位点数量。它能解释 percent identity 为什么降低，也能在总比对长度较长时提示哪些命中存在较多分化区域。",
		"アラインメント内の不一致位置数です。percent identity が低い理由を説明し、長いアラインメントでも分化した領域が多い hit を見つける助けになります。",
	),
	"gap_openings": columnHelp(
		"Number of gap-opening events in the alignment. Many gap openings can indicate insertions, deletions, assembly issues, annotation problems, or a hit that only partially preserves the expected protein architecture.",
		"比对中 gap opening 的数量。大量 gap opening 可能提示插入、缺失、组装问题、注释问题，或 hit 只部分保留预期蛋白结构。",
		"アラインメント中の gap opening 数です。多数の gap opening は挿入、欠失、アセンブリ問題、注釈問題、または期待されるタンパク質構造が部分的にしか保存されていないことを示す場合があります。",
	),
	"identical": columnHelp(
		"Count of identical positions in the alignment. This is the raw numerator behind percent identity and is useful when comparing alignments with very different lengths.",
		"比对中完全相同的位置数量。它是 percent identity 背后的原始分子，适合在比较长度差异很大的比对时参考。",
		"アラインメント中で完全一致した位置数です。percent identity の元になる実数で、長さが大きく異なるアラインメントを比較するときに有用です。",
	),
	"positives": columnHelp(
		"Count of positive-scoring positions in protein alignments. These include conservative amino-acid substitutions, so positives can reveal functional similarity even when exact identity is moderate.",
		"蛋白比对中正得分的位置数量。它包含保守氨基酸替换，因此即使 exact identity 中等，positives 也可能显示功能相似性。",
		"タンパク質アラインメントで正のスコアを持つ位置数です。保存的アミノ酸置換を含むため、identity が中程度でも機能的類似性を示すことがあります。",
	),
	"gaps": columnHelp(
		"Number of gap positions in the alignment. High gap counts can reduce confidence in a full-length ortholog call and should be checked together with InterPro regions and source annotations.",
		"比对中的 gap 位点数量。gap 数较高会降低全长同源判断的可信度，需要与 InterPro 区域和原始注释一起检查。",
		"アラインメント内の gap 位置数です。gap が多い場合は全長オルソログ判定の信頼性が下がるため、InterPro 領域や元注釈と一緒に確認します。",
	),
	"jbrowse_name": columnHelp(
		"Genome identifier used by Phytozome/JBrowse links. It is mostly a technical field, but it helps construct or debug genome-browser URLs for visual inspection of the hit locus.",
		"Phytozome/JBrowse 链接使用的基因组编号。它主要是技术字段，但可用于构建或排查 genome browser URL，以便查看命中位点。",
		"Phytozome/JBrowse リンクで使うゲノム ID です。主に技術的な項目ですが、hit locus を視覚的に確認するための genome browser URL 作成や確認に役立ちます。",
	),
	"target_id": columnHelp(
		"Internal target proteome or database identifier used by the source database. It is useful for troubleshooting API calls, sequence fetching, and cases where visible protein IDs are not enough to retrieve records.",
		"原始数据库使用的内部目标蛋白组或数据库编号。它适合用于排查 API 调用、序列获取，以及可见 protein ID 不足以取回记录的情况。",
		"元データベースが使う内部 target proteome または database ID です。API 呼び出し、配列取得、表示 protein ID だけではレコードを取得できない場合の確認に役立ちます。",
	),
	"sequence_id": columnHelp(
		"Internal sequence identifier used by the source database for peptide retrieval. It can differ from public protein IDs, so it is kept to make sequence-fetching and export behavior reproducible.",
		"原始数据库用于获取肽序列的内部 sequence identifier。它可能不同于公开 protein ID，因此保留它可以让序列获取和导出行为可重复。",
		"元データベースがペプチド取得に使う内部 sequence ID です。公開 protein ID と異なる場合があるため、配列取得と出力を再現できるよう保持します。",
	),
	"transcript_id": columnHelp(
		"Transcript identifier associated with the target hit. It helps connect protein-level BLAST hits to gene models, isoforms, transcript reports, and genome-browser evidence.",
		"目标命中关联的转录本编号。它帮助把蛋白层面的 BLAST hit 连接到基因模型、isoform、转录本报告和 genome browser 证据。",
		"target hit に関連する transcript ID です。タンパク質レベルの BLAST hit を遺伝子モデル、isoform、transcript report、genome browser 証拠につなぎます。",
	),
	"defline": columnHelp(
		"Original sequence definition line or annotation text from the source database. This field often contains functional hints, predicted protein descriptions, isoform notes, and source-specific naming that may not appear in external references.",
		"原始数据库中的序列 defline 或注释文本。这个字段常包含功能线索、预测蛋白描述、isoform 说明以及外部参考库里不一定出现的源数据库命名。",
		"元データベースの sequence definition line または注釈テキストです。機能の手がかり、予測タンパク質説明、isoform 情報、外部参照に出ないソース固有の名称を含むことがあります。",
	),
	"gene_report_url": columnHelp(
		"Source database report URL for the hit. Open it when you need to inspect aliases, gene models, transcript/protein pages, annotation context, or browser links directly in the original database.",
		"该命中在原始数据库中的报告页面 URL。需要直接查看 alias、基因模型、转录本/蛋白页面、注释上下文或浏览器链接时使用。",
		"hit の元データベースレポート URL です。alias、遺伝子モデル、transcript/protein ページ、注釈文脈、browser link を直接確認したいときに使います。",
	),
	"protein_id": columnHelp(
		"Protein identifier from the source database record. In keyword results it is the stable handle used for peptide retrieval, external reference mapping, and later BLAST or export steps.",
		"源数据库记录中的蛋白编号。在 keyword 结果中，它是获取肽序列、映射外部参考库以及后续 BLAST 或导出的稳定标识。",
		"ソースデータベースレコードの protein ID です。keyword 結果では、ペプチド取得、外部参照マッピング、後続 BLAST/出力に使う安定した識別子です。",
	),
	"transcript": columnHelp(
		"Transcript or transcript-like identifier shown in keyword results. It helps distinguish isoforms and connect keyword hits to the source gene model.",
		"keyword 结果中显示的转录本或类似转录本编号。它帮助区分 isoform，并把 keyword 命中连接到原始基因模型。",
		"keyword 結果に表示される transcript または transcript 相当の ID です。isoform の区別や、keyword hit と元の遺伝子モデルの接続に役立ちます。",
	),
	"gene_identifier": columnHelp(
		"Gene-level identifier associated with the record. Use it when you want to group multiple transcripts or proteins back to the same gene locus.",
		"记录关联的基因层面编号。当需要把多个转录本或蛋白归回同一个基因位点时使用。",
		"レコードに関連する gene-level ID です。複数の transcript や protein を同じ gene locus にまとめたいときに使います。",
	),
	"genome": columnHelp(
		"Genome assembly or dataset label from the source database. It tells you which release the record belongs to and is useful when comparing results across database versions.",
		"源数据库中的基因组组装或数据集标签。它说明记录属于哪个 release，在跨数据库版本比较结果时很有用。",
		"ソースデータベースの genome assembly または dataset label です。レコードがどの release に属するかを示し、データベースバージョン間の比較に役立ちます。",
	),
	"gnome": columnHelp(
		"Displayed genome/species label used by the current table. The spelling is kept for compatibility with existing table definitions, but the meaning is genome or species context.",
		"当前表格使用的 genome/species 显示标签。字段名保留了既有表格定义中的拼写，但含义是基因组或物种上下文。",
		"現在の表で使われる genome/species 表示ラベルです。既存の表定義との互換性のためこの綴りを保持していますが、意味はゲノムまたは種の文脈です。",
	),
	"location": columnHelp(
		"Genomic location for the source record when available. It helps check whether candidate genes cluster, overlap expected loci, or need visual inspection in a genome browser.",
		"源记录的基因组位置（如果可用）。它可用于检查候选基因是否成簇、是否落在预期位点，或是否需要在 genome browser 中查看。",
		"利用可能な場合のソースレコードのゲノム位置です。候補遺伝子のクラスター、期待 locus との一致、genome browser での確認が必要かを判断できます。",
	),
	"aliases": columnHelp(
		"Aliases reported by the source database. The first alias is often used as an automatic label name, and the full alias list helps recognize familiar gene symbols such as pathway enzyme names.",
		"源数据库报告的 alias。第一项 alias 常用于自动 label name，完整 alias 列表可帮助识别熟悉的基因符号，例如通路酶名称。",
		"ソースデータベースが報告する alias です。最初の alias は自動 label name に使われることが多く、全 alias 一覧は経路酵素名など既知の gene symbol の認識に役立ちます。",
	),
	"uniprot": columnHelp(
		"Source-database UniProt cross-reference when the original database already provides one. This is separate from the added UniProt external-reference columns and can help diagnose mapping agreement or disagreement.",
		"原始数据库自身提供的 UniProt 交叉引用。它不同于后续添加的 UniProt 外部参考列，可用于检查映射是否一致。",
		"元データベースがすでに提供している UniProt cross-reference です。追加された UniProt 外部参照列とは別で、マッピングの一致/不一致を確認できます。",
	),
	"discripition": columnHelp(
		"Description or annotation text from the source database. The field name keeps the existing table spelling, but the content is the source description used for quick functional interpretation.",
		"源数据库中的描述或注释文本。字段名保留既有表格拼写，但内容是用于快速功能判断的源数据库 description。",
		"ソースデータベースの description または注釈テキストです。フィールド名は既存表の綴りを保持していますが、内容は機能を素早く解釈するための説明です。",
	),
	"comments": columnHelp(
		"Additional source-database comments for the record. These may include notes that are not part of the main description but can still clarify annotation quality or special cases.",
		"该记录的源数据库附加 comments。这些内容可能不属于主要 description，但仍可说明注释质量或特殊情况。",
		"レコードに対するソースデータベースの追加 comments です。主要 description ではないものの、注釈品質や特殊ケースを理解する助けになる場合があります。",
	),
	"auto_define": columnHelp(
		"Program-generated definition or label derived from available source information. It is a convenience field for readable output and should be checked against source aliases, descriptions, and external references.",
		"程序根据可用源信息生成的定义或标签。这是为了让输出更可读的便利字段，需要与源 alias、description 和外部参考信息一起确认。",
		"利用可能なソース情報からプログラムが生成した定義またはラベルです。出力を読みやすくするための補助項目であり、source aliases、description、外部参照と照合して確認します。",
	),
	"sequence_header_label": columnHelp(
		"Header label used when this row's sequence is exported or reused. It is designed to keep exported FASTA records identifiable in downstream BLAST, alignment, or phylogenetic tools.",
		"该行序列导出或复用时使用的 header label。它用于让导出的 FASTA 记录在后续 BLAST、比对或系统发育工具中保持可识别。",
		"この行の配列を出力または再利用するときの header label です。下流の BLAST、アラインメント、系統解析ツールで FASTA レコードを識別しやすくします。",
	),
	"uniprot_accession": columnHelp(
		"UniProtKB accession mapped to this hit by the external UniProt reference step. It identifies the specific UniProt entry used for standardized names, canonical length, GO, EC, domains, and other reference annotations.",
		"外部 UniProt 参考步骤映射到该 hit 的 UniProtKB accession。它标识用于标准名称、canonical length、GO、EC、结构域等参考注释的具体 UniProt 条目。",
		"外部 UniProt 参照ステップでこの hit にマップされた UniProtKB accession です。標準名、canonical length、GO、EC、ドメインなどの参照注釈に使った UniProt エントリーを示します。",
	),
	"uniprot_entry_name": columnHelp(
		"UniProt entry name, a compact UniProt identifier that often combines a protein mnemonic and organism code. It is useful for quickly recognizing reviewed model entries and comparing hits in exported tables.",
		"UniProt entry name，是通常由蛋白缩写和物种代码组成的紧凑 UniProt 标识。它适合快速识别 reviewed 模型条目，并在导出表格中比较 hit。",
		"UniProt entry name は、タンパク質 mnemonic と organism code を組み合わせた短い UniProt ID です。reviewed なモデルエントリーの認識や出力表での比較に役立ちます。",
	),
	"uniprot_reviewed": columnHelp(
		"UniProt review status. reviewed means a Swiss-Prot curated entry with stronger manual annotation support; unreviewed means a TrEMBL computationally annotated entry that can still be useful but should be interpreted more cautiously.",
		"UniProt 审核状态。reviewed 表示 Swiss-Prot 人工整理条目，注释支持通常更强；unreviewed 表示 TrEMBL 自动注释条目，仍有用但需要更谨慎解释。",
		"UniProt の review status です。reviewed は Swiss-Prot の手動 curated entry で注釈信頼度が高く、unreviewed は TrEMBL の計算注釈 entry で有用ですが慎重に解釈します。",
	),
	"uniprot_protein_name": columnHelp(
		"Recommended or submitted protein name reported by UniProt. This is often more standardized than source-database deflines and helps recognize enzyme families, transporters, transcription factors, and pathway proteins.",
		"UniProt 报告的推荐或提交蛋白名称。它通常比原始数据库 defline 更标准，有助于识别酶家族、转运蛋白、转录因子和通路蛋白。",
		"UniProt が報告する recommended または submitted protein name です。元データベースの defline より標準化されていることが多く、酵素ファミリー、輸送体、転写因子、経路タンパク質の認識に役立ちます。",
	),
	"uniprot_gene_names": columnHelp(
		"Gene names reported by UniProt, including primary and synonym names when available. They are useful for recognizing pathway enzyme symbols, family aliases, and alternative gene names without changing the source-database label.",
		"UniProt 报告的基因名称，包括可用的主名和同义名。它们有助于识别通路酶符号、家族别名和其他基因名，同时不会改变原始数据库的 label。",
		"UniProt が報告する gene name です。利用可能な primary name と synonym を含みます。経路酵素記号、ファミリー別名、別名遺伝子名の確認に役立ちますが、元データベースの label は変更しません。",
	),
	"uniprot_organism": columnHelp(
		"Organism name in the mapped UniProt entry. Compare it with the source species to detect cross-species mappings, close homolog mappings, or cases where UniProt has no exact species-specific entry.",
		"映射到的 UniProt 条目中的物种名称。可与原始 species 对比，用来发现跨物种映射、近缘同源映射，或 UniProt 没有精确物种条目的情况。",
		"マップされた UniProt エントリーの organism name です。source species と比較することで、種をまたぐマッピング、近縁 homolog へのマッピング、UniProt に厳密な種別エントリーがない場合を検出できます。",
	),
	"uniprot_organism_id": columnHelp(
		"NCBI taxonomy identifier from the UniProt entry. It provides a stable taxonomy handle for checking organism identity and for grouping results across species.",
		"UniProt 条目中的 NCBI taxonomy 编号。它提供稳定的分类学标识，可用于检查物种身份并跨物种分组结果。",
		"UniProt エントリーの NCBI taxonomy ID です。生物種の確認や種をまたぐ結果のグループ化に使える安定した分類 ID です。",
	),
	"uniprot_keywords": columnHelp(
		"Functional and biological keywords assigned by UniProt. They provide quick tags for processes, molecular functions, enzyme classes, locations, or annotation flags and are useful for manual review even when not shown in the compact table view.",
		"UniProt 分配的功能与生物学关键词。它们提供过程、分子功能、酶类型、定位或注释标记的快速标签，即使不在紧凑表格中显示，也适合详情和导出时人工检查。",
		"UniProt が付与した機能・生物学キーワードです。process、molecular function、enzyme class、location、annotation flag の素早いタグとして使え、コンパクト表に出ない場合でも詳細確認や出力で有用です。",
	),
	"uniprot_ec": columnHelp(
		"Enzyme Commission number from UniProt. EC numbers are especially important for metabolic pathway proteins because they connect hits to catalyzed reaction classes and pathway steps.",
		"UniProt 提供的 EC 酶编号。EC 对代谢通路蛋白尤其重要，因为它把 hit 连接到催化反应类别和通路步骤。",
		"UniProt の Enzyme Commission number です。EC 番号は代謝経路タンパク質で特に重要で、hit を触媒反応クラスや経路ステップへ結び付けます。",
	),
	"uniprot_go": columnHelp(
		"Gene Ontology terms from UniProt. These terms summarize molecular function, biological process, and cellular component evidence and can help compare predicted roles across species.",
		"UniProt 提供的 Gene Ontology 术语。它们概括分子功能、生物过程和细胞组分证据，可用于跨物种比较预测功能。",
		"UniProt の Gene Ontology term です。molecular function、biological process、cellular component の証拠を要約し、種間で予測役割を比較する助けになります。",
	),
	"uniprot_go_ids": columnHelp(
		"Stable GO identifiers from UniProt. IDs are safer than names for programmatic comparison because GO term names can be updated while identifiers remain stable.",
		"UniProt 提供的稳定 GO 编号。与名称相比，ID 更适合程序化比较，因为 GO term 名称可能更新，而编号更稳定。",
		"UniProt の安定した GO ID です。GO term 名は更新される可能性があるため、プログラムで比較する場合は ID の方が安全です。",
	),
	"uniprot_function": columnHelp(
		"UniProt function comment. For reviewed entries it may contain concise curated functional evidence; for unreviewed entries it can still provide useful predicted roles but should be checked against BLAST and InterPro evidence.",
		"UniProt 的 function 注释。reviewed 条目中可能包含简洁的人工整理功能证据；unreviewed 条目中也可能提供有用预测，但需要与 BLAST 和 InterPro 证据交叉检查。",
		"UniProt の function comment です。reviewed entry では curated な機能証拠を含む場合があり、unreviewed entry でも有用な予測を含みますが BLAST と InterPro 証拠と照合します。",
	),
	"uniprot_catalytic_activity": columnHelp(
		"UniProt catalytic activity comment, often including reactions, Rhea IDs, ChEBI compounds, and EC evidence. It is one of the most informative fields for pathway-enzyme interpretation.",
		"UniProt 的 catalytic activity 注释，通常包含反应、Rhea ID、ChEBI 化合物和 EC 证据。它是解释通路酶功能时最有信息量的字段之一。",
		"UniProt の catalytic activity comment です。reaction、Rhea ID、ChEBI compound、EC 証拠を含むことが多く、経路酵素の解釈で非常に有用な項目です。",
	),
	"uniprot_pathway": columnHelp(
		"Pathway comment from UniProt. It can directly mention metabolic or biological pathways and is useful for connecting a hit to pathway context before dedicated pathway databases are added.",
		"UniProt 的 pathway 注释。它可能直接提到代谢或生物学通路，在专门通路数据库接入前，可用于把 hit 连接到通路上下文。",
		"UniProt の pathway comment です。代謝経路や生物学的経路に直接触れる場合があり、専用 pathway database を追加する前でも hit を経路文脈へつなげられます。",
	),
	"uniprot_subcellular_location": columnHelp(
		"Subcellular location comment from UniProt. Localization can help evaluate functional plausibility, for example whether a candidate enzyme is predicted in the compartment expected for a pathway step.",
		"UniProt 的亚细胞定位注释。定位可帮助判断功能可信度，例如候选酶是否位于某个通路步骤预期的细胞区室。",
		"UniProt の subcellular location comment です。局在は機能的妥当性の評価に役立ち、候補酵素が経路ステップで期待される区画にあるか確認できます。",
	),
	"uniprot_protein_existence": columnHelp(
		"UniProt protein-existence evidence level. It indicates whether the protein is supported by protein-level evidence, transcript evidence, homology, prediction, or uncertainty.",
		"UniProt 的蛋白存在证据等级。它说明该蛋白是否有蛋白水平证据、转录本证据、同源证据、预测证据或不确定证据支持。",
		"UniProt の protein existence evidence level です。タンパク質レベル、転写産物、相同性、予測、不確実性のどの証拠に支えられるかを示します。",
	),
	"uniprot_annotation_score": columnHelp(
		"UniProt annotation score. Higher scores generally indicate richer annotation, but the score is not a direct measure of whether this BLAST hit is the correct functional ortholog.",
		"UniProt 注释评分。较高分通常表示注释更丰富，但该分数不能直接证明这个 BLAST hit 就是正确的功能同源基因。",
		"UniProt annotation score です。高いほど注釈が豊富な傾向がありますが、この BLAST hit が正しい機能的オルソログであることを直接示すものではありません。",
	),
	"uniprot_fragment": columnHelp(
		"Indicates whether UniProt marks the mapped sequence as a fragment. Fragment entries should be treated carefully when using canonical length, domain completeness, or pathway-enzyme interpretation.",
		"表示 UniProt 是否将映射序列标记为片段。使用 canonical length、结构域完整性或通路酶功能解释时，fragment 条目需要谨慎处理。",
		"マップされた配列が UniProt で fragment とされているかを示します。canonical length、ドメイン完全性、経路酵素解釈に使う場合は慎重に扱います。",
	),
	"uniprot_sequence_caution": columnHelp(
		"UniProt sequence caution notes. These warnings may mention frameshifts, conflicts, uncertain sequence regions, or other issues that affect confidence in length and domain interpretation.",
		"UniProt 的序列警告说明。这些 warning 可能提到 frameshift、冲突、不确定序列区域或其他会影响长度和结构域解释可信度的问题。",
		"UniProt の sequence caution notes です。frameshift、conflict、不確実な配列領域など、長さやドメイン解釈の信頼性に影響する問題を含む場合があります。",
	),
	"uniprot_pfam": columnHelp(
		"Pfam cross-references from UniProt. They provide an alternate route to conserved-domain evidence and can be compared with InterPro-derived Pfam accessions.",
		"UniProt 中的 Pfam 交叉引用。它们提供另一条保守结构域证据来源，可与 InterPro 派生的 Pfam accession 比较。",
		"UniProt の Pfam cross-reference です。保存ドメイン証拠への別ルートであり、InterPro 由来の Pfam accession と比較できます。",
	),
	"uniprot_interpro": columnHelp(
		"InterPro cross-references from UniProt. These are useful for comparing UniProt's cross-reference view with the direct InterPro API lookup performed by this program.",
		"UniProt 中的 InterPro 交叉引用。它们可用于比较 UniProt 交叉引用视图与本程序直接调用 InterPro API 得到的结果。",
		"UniProt の InterPro cross-reference です。このプログラムが直接 InterPro API で取得した結果と、UniProt 側の cross-reference view を比較できます。",
	),
	"uniprot_domain": columnHelp(
		"UniProt feature-table domain annotations. These are curated or predicted domain features on the UniProt sequence and can help interpret whether the BLAST hit preserves important functional architecture.",
		"UniProt feature table 中的 domain 注释。这些是 UniProt 序列上的人工整理或预测结构域特征，可帮助判断 BLAST hit 是否保留重要功能结构。",
		"UniProt feature table の domain annotation です。UniProt 配列上の curated または predicted domain feature で、BLAST hit が重要な機能構造を保持するか判断できます。",
	),
	"uniprot_region": columnHelp(
		"UniProt feature-table region annotations. Regions may describe functional segments, low-complexity regions, compositional bias, repeats, or other biologically meaningful spans.",
		"UniProt feature table 中的 region 注释。region 可能描述功能片段、低复杂度区域、组成偏倚、repeat 或其他有生物学意义的跨度。",
		"UniProt feature table の region annotation です。機能セグメント、低複雑性領域、組成バイアス、リピート、その他の生物学的に意味のある範囲を示す場合があります。",
	),
	"uniprot_motif": columnHelp(
		"UniProt motif feature annotations. Motifs can indicate short conserved functional patterns and should be checked when a pathway enzyme depends on a small catalytic or binding motif.",
		"UniProt motif 特征注释。motif 可提示短的保守功能模式；当通路酶依赖小的催化或结合 motif 时尤其值得检查。",
		"UniProt の motif feature annotation です。短い保存機能パターンを示すことがあり、経路酵素が小さな触媒/結合 motif に依存する場合は特に確認します。",
	),
	"uniprot_active_site": columnHelp(
		"UniProt active-site feature annotations. These are high-value functional residues; if a candidate lacks the expected active-site region, it may be a paralog, pseudogene, fragment, or unrelated hit.",
		"UniProt active site 特征注释。这些是价值很高的功能残基；如果候选缺少预期 active-site 区域，可能是旁系同源、假基因、片段或无关命中。",
		"UniProt の active-site feature annotation です。重要な機能残基を示します。候補が期待される active-site 領域を欠く場合、paralog、pseudogene、fragment、無関係な hit の可能性があります。",
	),
	"uniprot_binding_site": columnHelp(
		"UniProt binding-site feature annotations. These can identify residues or regions involved in ligand, metal, cofactor, DNA, or substrate binding, depending on the entry.",
		"UniProt binding site 特征注释。根据条目不同，它们可标识参与配体、金属、辅因子、DNA 或底物结合的残基或区域。",
		"UniProt の binding-site feature annotation です。entry によって、リガンド、金属、補因子、DNA、基質結合に関わる残基や領域を示します。",
	),
	"uniprot_alphafolddb": columnHelp(
		"AlphaFoldDB cross-reference from UniProt. It points to predicted structural models when available, useful for later structural inspection of conserved domains or active-site geometry.",
		"UniProt 中的 AlphaFoldDB 交叉引用。可指向可用的预测结构模型，适合后续查看保守结构域或 active-site 几何结构。",
		"UniProt の AlphaFoldDB cross-reference です。利用可能な予測構造モデルを指し、保存ドメインや active-site geometry の構造確認に役立ちます。",
	),
	"uniprot_pdb": columnHelp(
		"PDB structure cross-references from UniProt. Experimental structures, when present, provide strong support for domain architecture and functional residue interpretation.",
		"UniProt 中的 PDB 结构交叉引用。如果有实验结构，它们能为结构域构成和功能残基解释提供强支持。",
		"UniProt の PDB structure cross-reference です。実験構造がある場合、ドメイン構造や機能残基の解釈を強く支えます。",
	),
	"uniprot_canonical_length": columnHelp(
		"Canonical protein sequence length reported by UniProt. It is used as a standardized length reference for judging whether the original database target length is reasonable, truncated, extended, or likely mapped to a different isoform.",
		"UniProt 报告的 canonical 蛋白序列长度。它作为标准长度参考，用于判断原始数据库 target length 是否合理、截短、延长或可能映射到不同 isoform。",
		"UniProt が報告する canonical protein sequence length です。元データベースの target length が妥当か、短縮/延長か、別 isoform にマップされた可能性があるかを判断する標準長として使います。",
	),
}

func blastTableHeader(text string) string {
	return ColumnCompactHeader(text, ColumnDisplayOptions{Multiline: true})
}

func blastAlignQueryLengthPercent(row model.BlastResultRow) string {
	if row.AlignQueryLengthPercent != 0 {
		return fmt.Sprintf("%.2f", row.AlignQueryLengthPercent)
	}
	if row.QueryLength <= 0 {
		return ""
	}
	return fmt.Sprintf("%.2f", float64(row.AlignLength)/float64(row.QueryLength)*100)
}

var ErrBackToDatabaseSelection = errors.New("back to database selection")
var ErrBackToModeSelection = errors.New("back to mode selection")
var ErrBackToSpeciesSelection = errors.New("back to species selection")
var ErrBackToQueryInput = errors.New("back to query input")
var ErrBackToBlastProgram = errors.New("back to BLAST program selection")
var ErrBackToRowSelection = errors.New("back to row selection")
var ErrDialogClosed = errors.New("dialog closed")
var ErrAutoIdentifyRequested = errors.New("auto identify protein labels")
var ErrExitRequested = errors.New("exit requested")

type KeywordRowSelection struct {
	Rows         []model.KeywordResultRow
	GenerateFile bool
}

type BlastRowSelection struct {
	Rows             []model.BlastResultRow
	GenerateFile     bool
	DoneAll          bool
	RunIndex         int
	Selected         []bool
	SelectedByRun    [][]bool
	RowsByRun        [][]model.BlastResultRow
	RowNumbers       []int
	RowNumbersByRun  [][]int
	FilterFlags      []bool
	FilterFlagsByRun [][]bool
	FilterSettings   model.BlastFilterSettings
	FilterApplied    bool
	FilterCleared    bool
}

type BlastRunView struct {
	Item BlastQueryItemView
	Rows []model.BlastResultRow
}

type BlastQueryItemView struct {
	RawInput     string
	LabelName    string
	GeneID       string
	TranscriptID string
	ProteinID    string
	FamilyName   string
	MemberLabel  string
}

type ExportSettings struct {
	BaseName      string
	FolderName    string
	WriteReport   bool
	WriteText     bool
	WriteExcel    bool
	WriteRawExcel bool
}

type BlastFilterSettingsResult struct {
	Settings    model.BlastFilterSettings
	ClearFilter bool
}

type ExternalReferenceSettings struct {
	UseUniProt       bool
	UseInterPro      bool
	InterProSettings model.InterProConservedRegionSettings
}

type BlastFilterRequest struct {
	Rows          []model.BlastResultRow
	RowsByRun     [][]model.BlastResultRow
	Selected      []bool
	SelectedByRun [][]bool
	Settings      model.BlastFilterSettings
	CurrentRun    int
	Profile       string
}

type BlastFilterSuggestion struct {
	Selected      []bool
	Flags         []bool
	SelectedByRun [][]bool
	FlagsByRun    [][]bool
	Settings      model.BlastFilterSettings
}

type blastFilterClearResult struct {
	Selected      []bool
	Flags         []bool
	SelectedByRun [][]bool
	FlagsByRun    [][]bool
}

type blastFilterEvaluation struct {
	Reject bool
	Score  int
	EValue float64
}

type tableColumnValue[T any] struct {
	ID        string
	Header    string
	Sortable  bool
	Reference string
	Help      string
	Value     func(T) string
}

func New(in io.Reader, out io.Writer) *Prompter {
	return &Prompter{
		out:                 out,
		rowStates:           make(map[string]tui.RowSelectionState),
		blastRunStates:      make(map[string]tui.BlastRunSelectionState),
		blastSelections:     make(map[string][]bool),
		blastRunSelected:    make(map[string][][]bool),
		blastFilterSettings: model.DefaultBlastFilterSettings(),
		blastFilterFlags:    make(map[string][]bool),
		blastRunFilterFlags: make(map[string][][]bool),
	}
}

func (p *Prompter) SetDatabaseContext(database string) {
	database = strings.TrimSpace(database)
	if database == "" {
		p.sessionPath = nil
		return
	}
	p.sessionPath = []string{database}
}

func (p *Prompter) SetBlastProgramContext(program string) {
	p.blastProgramPath = strings.TrimSpace(program)
}

func (p *Prompter) t(text string) string {
	return text
}

func (p *Prompter) tf(text string, args ...any) string {
	return fmt.Sprintf(text, args...)
}

func tuiNavError(nav tui.NavAction, backTarget error) error {
	switch nav {
	case tui.NavNone:
		return nil
	case tui.NavBack:
		if backTarget != nil {
			return backTarget
		}
		return nil
	case tui.NavHome:
		return ErrBackToDatabaseSelection
	case tui.NavExit:
		return ErrExitRequested
	default:
		return nil
	}
}

func (p *Prompter) tuiPath(parts ...string) []string {
	path := make([]string, 0, len(parts)+1+len(p.sessionPath))
	path = append(path, "phytozome GO")
	path = append(path, p.sessionPath...)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			path = append(path, part)
		}
	}
	return path
}

func (p *Prompter) blastTUIPath(parts ...string) []string {
	base := []string{"Startup", "Species", "BLAST program selection"}
	if strings.TrimSpace(p.blastProgramPath) != "" {
		base = append(base, p.blastProgramPath)
	}
	base = append(base, parts...)
	return p.tuiPath(base...)
}

func firstNonEmptyText(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (p *Prompter) ChooseDatabase() (string, error) {
	result, err := tui.RunChoicePage(tui.ChoicePage{
		Path:        p.tuiPath("Startup", "Database"),
		Title:       p.t("Database selection:"),
		Description: p.t("Choose the data source for this session."),
		Choices: []tui.Choice{
			{Value: "phytozome", Label: "phytozome", Description: p.t("original Phytozome workflow")},
			{Value: "lemna", Label: "lemna", Description: p.t("lemna.org download-backed workflow")},
		},
		AllowBack:   false,
		AllowHome:   false,
		ConfirmText: tui.ButtonSelect,
	})
	if err != nil {
		return "", err
	}
	if navErr := tuiNavError(result.Nav, ErrBackToDatabaseSelection); navErr != nil {
		return "", navErr
	}
	if result.Value == "" {
		return "", ErrExitRequested
	}
	return result.Value, nil
}

func (p *Prompter) ChooseMode() (string, error) {
	result, err := tui.RunChoicePage(tui.ChoicePage{
		Path:        p.tuiPath("Startup", "Mode"),
		Title:       p.t("Mode selection:"),
		Description: p.t("Choose what kind of search to run."),
		Choices: []tui.Choice{
			{Value: "blast", Label: "blast", Description: p.t("sequence / FASTA / URL query against one species")},
			{Value: "keyword", Label: "keyword", Description: p.t("keyword gene search within one species")},
		},
		AllowBack:   true,
		AllowHome:   true,
		ConfirmText: tui.ButtonSelect,
	})
	if err != nil {
		return "", err
	}
	if navErr := tuiNavError(result.Nav, ErrBackToDatabaseSelection); navErr != nil {
		return "", navErr
	}
	if result.Value == "" {
		return "", ErrExitRequested
	}
	return result.Value, nil
}

// ChooseBlastProgram prompts the user to pick one BLAST program from the
// provided list of program names. The prompt accepts either a program number
// (1-based) or the program name (case-insensitive). Returns the selected
// program string as given in the `programs` slice.
func (p *Prompter) ChooseBlastProgram(programs []string) (string, error) {
	defaultProgram := ""
	if len(programs) > 0 {
		defaultProgram = programs[0]
	}
	for _, program := range programs {
		if strings.EqualFold(strings.TrimSpace(program), "blastp") {
			defaultProgram = program
			break
		}
	}
	groups := make([]tui.ChoiceGroup, 0, 3)
	currentGroup := -1
	addGroup := func(label string, description string) {
		groups = append(groups, tui.ChoiceGroup{
			Label:       p.t(label),
			Description: p.t(description),
			Choices:     []tui.Choice{},
		})
		currentGroup = len(groups) - 1
	}
	addProgram := func(program string) {
		for _, candidate := range programs {
			if strings.EqualFold(candidate, program) {
				if currentGroup < 0 {
					addGroup("Other programs", "Additional BLAST programs detected for this species.")
				}
				label := candidate
				description := p.t(blastProgramDescription(candidate))
				if strings.EqualFold(candidate, defaultProgram) {
					label += " (default)"
					description = strings.TrimSpace(description + " | default")
				}
				groups[currentGroup].Choices = append(groups[currentGroup].Choices, tui.Choice{
					Value:       candidate,
					Label:       label,
					Description: description,
				})
				return
			}
		}
	}
	addGroup("Start with nucleotide", "Use when your query is DNA/RNA or a nucleotide FASTA sequence.")
	addProgram("blastn")
	addProgram("blastx")
	addGroup("Start with protein", "Use when your query is an amino-acid/protein sequence.")
	addProgram("tblastn")
	addProgram("blastp")
	for _, program := range programs {
		found := false
		for _, group := range groups {
			for _, choice := range group.Choices {
				if strings.EqualFold(choice.Value, program) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			if len(groups) == 0 || groups[len(groups)-1].Label != p.t("Other programs") {
				addGroup("Other programs", "Additional BLAST programs detected for this species.")
			}
			label := program
			description := p.t(blastProgramDescription(program))
			if strings.EqualFold(program, defaultProgram) {
				label += " (default)"
				description = strings.TrimSpace(description + " | default")
			}
			groups[len(groups)-1].Choices = append(groups[len(groups)-1].Choices, tui.Choice{Value: program, Label: label, Description: description})
		}
	}
	result, err := tui.RunGroupedChoicePage(tui.GroupedChoicePage{
		Path:        p.tuiPath("Startup", "Species", "BLAST program selection"),
		Title:       p.t("BLAST program selection:"),
		Description: p.t("Programs are grouped by query type. Choose the program that matches your query and target database."),
		Groups:      groups,
		Initial:     defaultProgram,
		AllowBack:   true,
		AllowHome:   true,
		ConfirmText: tui.ButtonSelect,
		Hints:       []string{p.t("Up/Down choose item | Number keys select item"), p.t("Enter confirms the highlighted option.")},
	})
	if err != nil {
		return "", err
	}
	if navErr := tuiNavError(result.Nav, ErrBackToSpeciesSelection); navErr != nil {
		return "", navErr
	}
	if result.Value == "" {
		return "", ErrExitRequested
	}
	return result.Value, nil

}

func blastProgramDescription(program string) string {
	switch strings.ToLower(strings.TrimSpace(program)) {
	case "blastn":
		return "nucleotide query -> nucleotide/genome database"
	case "blastx":
		return "nucleotide query -> translated protein -> protein database"
	case "tblastn":
		return "protein query -> translated nucleotide/genome database"
	case "blastp":
		return "protein query -> protein database"
	default:
		return "BLAST search program"
	}
}

// ChooseBlastExecution asks whether to run the BLAST job on the server or
// locally. Returns \"server\" or \"local\". Accepts numeric choice or name.
func (p *Prompter) ChooseBlastExecution() (string, error) {
	result, err := tui.RunChoicePage(tui.ChoicePage{
		Path:        p.tuiPath("Startup", "Species", "BLAST program selection", "BLAST execution target"),
		Title:       p.t("BLAST execution target:"),
		Description: p.t("Choose where this BLAST job should run."),
		Choices: []tui.Choice{
			{Value: "server", Label: "server", Description: p.t("try the remote lemna.org BLAST service first")},
			{Value: "local", Label: "local", Description: p.t("download FASTA files automatically and run BLAST on this computer")},
		},
		AllowBack:   true,
		AllowHome:   true,
		ConfirmText: tui.ButtonSelect,
		Hints:       []string{p.t("Local mode does not require you to prepare the FASTA files yourself."), p.t("It does require NCBI BLAST+ on PATH, including makeblastdb.")},
	})
	if err != nil {
		return "", err
	}
	if navErr := tuiNavError(result.Nav, ErrBackToBlastProgram); navErr != nil {
		return "", navErr
	}
	if result.Value != "" {
		return result.Value, nil
	}
	return "", ErrExitRequested

}

func (p *Prompter) SpeciesKeyword() (string, error) {
	result, err := tui.RunTextInputPage(tui.TextInputPage{
		Path:        p.tuiPath("Startup", "Species search"),
		Title:       p.t("Species search"),
		Description: p.t("Enter a partial species keyword such as 'spiro', 'wheat', or 'arabidopsis'."),
		Label:       p.t("Keyword"),
		AllowEmpty:  true,
		AllowBack:   true,
		AllowHome:   true,
		ConfirmText: tui.ButtonSearch,
		Hints:       []string{p.t("You can search by abbreviated name, full scientific name, or common name.")},
	})
	if err != nil {
		return "", err
	}
	if navErr := tuiNavError(result.Nav, ErrBackToModeSelection); navErr != nil {
		return "", navErr
	}
	return result.Text, nil

}

func (p *Prompter) SelectSpecies(candidates []model.SpeciesCandidate) (model.SpeciesCandidate, error) {
	choices := make([]tui.Choice, 0, len(candidates))
	for i, candidate := range candidates {
		choices = append(choices, tui.Choice{
			Value:       strconv.Itoa(i),
			Label:       candidate.DisplayLabel(),
			Description: strings.TrimSpace(candidate.JBrowseName + targetIDLabel(candidate.ProteomeID)),
		})
	}
	result, err := tui.RunChoicePage(tui.ChoicePage{
		Path:        p.tuiPath("Startup", "Species selection"),
		Title:       p.t("Species selection"),
		Description: p.t("Select one species from the loaded candidates."),
		Choices:     choices,
		AllowBack:   true,
		AllowHome:   true,
		ConfirmText: tui.ButtonSelect,
		Hints:       []string{p.t("Up/Down choose item | Number keys select item")},
	})
	if err != nil {
		return model.SpeciesCandidate{}, err
	}
	if navErr := tuiNavError(result.Nav, ErrBackToModeSelection); navErr != nil {
		return model.SpeciesCandidate{}, navErr
	}
	index, err := strconv.Atoi(result.Value)
	if err != nil || index < 0 || index >= len(candidates) {
		return model.SpeciesCandidate{}, ErrExitRequested
	}
	return candidates[index], nil

}

func (p *Prompter) KeywordLabelNames(termCount int, backTarget error) ([]string, error) {
	for {
		result, err := tui.RunMultiLinePage(tui.MultiLinePage{
			Path:          p.tuiPath("Startup", "Keyword", "Label names"),
			Title:         p.t("Label names:"),
			Description:   fmt.Sprintf("Enter exactly %d label names, one per line. Use ~ for a blank label, or leave this page empty to skip labels.", termCount),
			AllowEmpty:    true,
			SkipWhenEmpty: true,
			AllowBack:     true,
			AllowHome:     true,
			ConfirmText:   tui.ButtonApply,
			EmptyText:     tui.ButtonAuto,
			EmptyAction:   "auto",
			SkipText:      tui.ButtonSkip,
			SkipShortcut:  tui.ShortcutRetry,
			Hints:         []string{p.t("The label names stay aligned with the search terms in order.")},
		})
		if err != nil {
			return nil, err
		}
		if navErr := tuiNavError(result.Nav, backTarget); navErr != nil {
			return nil, navErr
		}
		if result.Action == "auto" {
			return nil, ErrAutoIdentifyRequested
		}
		if result.Action == "skip" || strings.TrimSpace(result.Text) == "" {
			return nil, nil
		}
		values := parseKeywordIdentityValues(strings.Split(result.Text, "\n"))
		if len(values) == termCount {
			return values, nil
		}
		if _, err := p.recoveryErrorAction(
			p.tuiPath("Startup", "Keyword", "Label names", "Validation"),
			p.t("Label names:"),
			fmt.Sprintf("Need exactly %d label names, got %d. Please re-enter.", termCount, len(values)),
			false,
			backTarget,
		); err != nil {
			return nil, err
		}
	}

}

func (p *Prompter) BlastLabelNames(itemCount int, required bool, backTarget error) ([]string, error) {
	for {
		description := fmt.Sprintf("Enter exactly %d label names, one per line. Use ~ for a blank label.", itemCount)
		if itemCount == 1 && !required {
			description = p.t("Enter one label for this BLAST query, or leave this page empty to skip.")
		}
		result, err := tui.RunMultiLinePage(tui.MultiLinePage{
			Path:          p.blastTUIPath("BLAST input", "Label names"),
			Title:         p.t("Label names:"),
			Description:   description,
			AllowEmpty:    true,
			SkipWhenEmpty: true,
			AllowBack:     true,
			AllowHome:     true,
			ConfirmText:   tui.ButtonApply,
			EmptyText:     tui.ButtonAuto,
			EmptyAction:   "auto",
			SkipText:      tui.ButtonSkip,
			SkipShortcut:  tui.ShortcutRetry,
			Hints:         []string{p.t("The labels stay aligned with the BLAST queries in order.")},
		})
		if err != nil {
			return nil, err
		}
		if navErr := tuiNavError(result.Nav, backTarget); navErr != nil {
			return nil, navErr
		}
		if result.Action == "auto" {
			return nil, ErrAutoIdentifyRequested
		}
		if (result.Action == "skip" || strings.TrimSpace(result.Text) == "") && !required {
			return make([]string, itemCount), nil
		}
		values := parseBlastIdentityValues(strings.Split(result.Text, "\n"))
		if len(values) == itemCount {
			return values, nil
		}
		if _, err := p.recoveryErrorAction(
			p.blastTUIPath("BLAST input", "Label names", "Validation"),
			p.t("Label names:"),
			fmt.Sprintf("Need exactly %d label names, got %d. Please re-enter.", itemCount, len(values)),
			false,
			backTarget,
		); err != nil {
			return nil, err
		}
	}

}

func (p *Prompter) OutputFolderName(backTarget error) (string, error) {
	result, err := tui.RunTextInputPage(tui.TextInputPage{
		Path:          p.blastTUIPath("BLAST input", "Output folder"),
		Title:         p.t("Output folder"),
		Description:   p.t("A folder name keeps all generated files together. Leave it blank to write files next to the program."),
		Label:         p.t("Folder"),
		AllowEmpty:    true,
		SkipWhenEmpty: true,
		AllowBack:     true,
		AllowHome:     true,
		ConfirmText:   tui.ButtonApply,
	})
	if err != nil {
		return "", err
	}
	if navErr := tuiNavError(result.Nav, backTarget); navErr != nil {
		return "", navErr
	}
	return strings.TrimSpace(result.Text), nil

}

func (p *Prompter) DetailedReportAction(backTarget error) (string, error) {
	result, err := tui.RunActionModalPage(tui.ActionModalPage{
		Path:    p.tuiPath("Startup", "Export", "Data analysis report"),
		Title:   p.t("Data analysis report (PDF)?"),
		Message: p.t("Choose whether to generate one PDF report for this export action."),
		Actions: []tui.Action{
			{Value: "close", Label: tui.ButtonClose},
			{Value: "no", Label: tui.ButtonNo},
		},
		ConfirmText:  tui.ButtonYes,
		ConfirmValue: "yes",
	})
	if err != nil {
		return "", err
	}
	if navErr := tuiNavError(result.Nav, backTarget); navErr != nil {
		return "", navErr
	}
	if result.Value == "close" {
		return "no", nil
	}
	if result.Value == "" {
		return "no", nil
	}
	return result.Value, nil

}

func (p *Prompter) ExternalReferenceSettings(backTarget error) (ExternalReferenceSettings, error) {
	defaultInterPro := model.DefaultInterProConservedRegionSettings()
	result, err := tui.RunExternalReferenceModal(tui.ExternalReferencePage{
		Path:            p.blastTUIPath("BLAST input", "External references"),
		Title:           p.t("External references"),
		Message:         p.t("Choose external reference databases used only to add columns to BLAST results."),
		UniProtLabel:    p.t("Use UniProt reference columns"),
		UniProtInitial:  true,
		InterProLabel:   p.t("Use InterPro reference columns"),
		InterProInitial: true,
		InterProSettings: tui.InterProConservedRegionSettings{
			UsePfamAccession:       defaultInterPro.UsePfamAccession,
			UseInterProAccession:   defaultInterPro.UseInterProAccession,
			UseSignatureAccession:  defaultInterPro.UseSignatureAccession,
			UseEntryType:           defaultInterPro.UseEntryType,
			UseEntryName:           defaultInterPro.UseEntryName,
			UseCoverage:            defaultInterPro.UseCoverage,
			UseMatchRegions:        defaultInterPro.UseMatchRegions,
			PresentMinCoverage:     fmt.Sprintf("%.0f", defaultInterPro.PresentMinCoverage),
			PartialMinCoverage:     fmt.Sprintf("%.0f", defaultInterPro.PartialMinCoverage),
			PresentMinMatchedItems: strconv.Itoa(defaultInterPro.PresentMinMatchedItems),
			PartialMinMatchedItems: strconv.Itoa(defaultInterPro.PartialMinMatchedItems),
		},
		AllowBack:   true,
		AllowHome:   true,
		ConfirmText: tui.ButtonApply,
	})
	if err != nil {
		return ExternalReferenceSettings{}, err
	}
	if navErr := tuiNavError(result.Nav, backTarget); navErr != nil {
		return ExternalReferenceSettings{}, navErr
	}
	return ExternalReferenceSettings{
		UseUniProt:       result.UseUniProt,
		UseInterPro:      result.UseInterPro,
		InterProSettings: parseInterProSettings(result.InterProSettings),
	}, nil
}

func (p *Prompter) FamilyBlastSettings(groups []FamilyBlastGroup, references model.FamilyBlastSettings, backTarget error) (model.FamilyBlastSettings, error) {
	defaults := model.DefaultFamilyBlastSettings()
	defaults.UseUniProtReference = references.UseUniProtReference
	defaults.UseInterProReference = references.UseInterProReference
	result, err := tui.RunFamilyBlastModal(tui.FamilyBlastPage{
		Path:      p.blastTUIPath("BLAST input", "Family BLAST"),
		Title:     p.t("Family BLAST"),
		Message:   p.t("Detected query groups that look like members of the same gene family. Family BLAST keeps the BLAST execution per query but reviews and exports each detected family as one grouped result."),
		Reference: familyBlastReferenceMessage(defaults.UseUniProtReference, defaults.UseInterProReference),
		Groups:    tuiFamilyBlastGroups(groups),
		Settings: tui.FamilyBlastSettings{
			Enabled:                    defaults.Enabled,
			GroupByDetectedPrefix:      defaults.GroupByDetectedPrefix,
			MergeRowsByTarget:          defaults.MergeRowsByTarget,
			KeepBestHitPerTarget:       defaults.KeepBestHitPerTarget,
			PrependOnlyFirstQuery:      defaults.PrependOnlyFirstQuery,
			MinimumGroupSize:           strconv.Itoa(defaults.MinimumGroupSize),
			StripArabidopsisPrefix:     defaults.StripArabidopsisPrefix,
			StripLeadingSpeciesPrefix:  defaults.StripLeadingSpeciesPrefix,
			StripTrailingQueryIndex:    defaults.StripTrailingQueryIndex,
			StripAfterNumberSuffix:     defaults.StripAfterNumberSuffix,
			NormalizeInnerPunctuation:  defaults.NormalizeInnerPunctuation,
			StripTerminalSubtypeSuffix: defaults.StripTerminalSubtypeSuffix,
			UseUniProtReference:        defaults.UseUniProtReference,
			UseInterProReference:       defaults.UseInterProReference,
			RankingTieBreakerOrder:     defaults.RankingTieBreakerOrder,
		},
		AllowBack:   true,
		AllowHome:   true,
		ConfirmText: tui.ButtonApply,
	})
	if err != nil {
		return model.FamilyBlastSettings{}, err
	}
	if navErr := tuiNavError(result.Nav, backTarget); navErr != nil {
		return model.FamilyBlastSettings{}, navErr
	}
	settings := result.Settings
	out := model.FamilyBlastSettings{
		Enabled:                    settings.Enabled,
		GroupByDetectedPrefix:      settings.GroupByDetectedPrefix,
		MergeRowsByTarget:          settings.MergeRowsByTarget,
		KeepBestHitPerTarget:       settings.KeepBestHitPerTarget,
		PrependOnlyFirstQuery:      settings.PrependOnlyFirstQuery,
		MinimumGroupSize:           parseIntDefault(settings.MinimumGroupSize, defaults.MinimumGroupSize),
		StripArabidopsisPrefix:     settings.StripArabidopsisPrefix,
		StripLeadingSpeciesPrefix:  settings.StripLeadingSpeciesPrefix,
		StripTrailingQueryIndex:    settings.StripTrailingQueryIndex,
		StripAfterNumberSuffix:     settings.StripAfterNumberSuffix,
		NormalizeInnerPunctuation:  settings.NormalizeInnerPunctuation,
		StripTerminalSubtypeSuffix: settings.StripTerminalSubtypeSuffix,
		UseUniProtReference:        defaults.UseUniProtReference,
		UseInterProReference:       defaults.UseInterProReference,
		RankingTieBreakerOrder:     settings.RankingTieBreakerOrder,
	}
	if out.MinimumGroupSize < 2 {
		out.MinimumGroupSize = 2
	}
	return out, nil
}

func familyBlastReferenceMessage(useUniProt bool, useInterPro bool) string {
	switch {
	case useUniProt && useInterPro:
		return "External references: UniProt + InterPro are enabled. Family BLAST will merge duplicate targets after both reference layers are added, and best-hit selection can use BLAST strength, UniProt review/length evidence, and InterPro conserved-region evidence."
	case useUniProt:
		return "External references: UniProt is enabled and InterPro is disabled. Family BLAST will merge duplicate targets with BLAST strength plus UniProt review and canonical-length evidence. InterPro conserved-region evidence and the automatic filter will not be available."
	case useInterPro:
		return "External references: InterPro is enabled and UniProt is disabled. Family BLAST will merge duplicate targets with BLAST strength plus InterPro conserved-region evidence. UniProt canonical-length evidence and the automatic filter will not be available."
	default:
		return "External references: none are enabled. Family BLAST will still group member queries, but duplicate-target best-hit selection uses BLAST columns only, and UniProt/InterPro evidence columns plus the automatic filter will not be available."
	}
}

type FamilyBlastGroup struct {
	Name    string
	Labels  []string
	Queries int
}

func tuiFamilyBlastGroups(groups []FamilyBlastGroup) []tui.FamilyBlastGroup {
	out := make([]tui.FamilyBlastGroup, 0, len(groups))
	for _, group := range groups {
		out = append(out, tui.FamilyBlastGroup{
			Name:    group.Name,
			Labels:  append([]string(nil), group.Labels...),
			Queries: group.Queries,
		})
	}
	return out
}

func (p *Prompter) BlastFilterSettings(backTarget error) (BlastFilterSettingsResult, error) {
	defaults := model.DefaultBlastFilterSettings()
	if p.blastFilterSettings == (model.BlastFilterSettings{}) {
		p.blastFilterSettings = defaults
	}
	result, err := tui.RunBlastFilterModal(tui.BlastFilterPage{
		Path:        p.blastTUIPath("BLAST results", "Filter"),
		Title:       p.t("BLAST filter"),
		Message:     p.t("Tune the automatic uncheck suggestion. The filter marks suggested removals in red but you can still edit row checkboxes afterward."),
		Settings:    tuiBlastFilterSettings(p.blastFilterSettings),
		AllowBack:   true,
		AllowHome:   true,
		ConfirmText: tui.ButtonFilter,
	})
	if err != nil {
		return BlastFilterSettingsResult{}, err
	}
	if navErr := tuiNavError(result.Nav, backTarget); navErr != nil {
		return BlastFilterSettingsResult{}, navErr
	}
	if result.ClearFilter {
		return BlastFilterSettingsResult{Settings: p.blastFilterSettings, ClearFilter: true}, nil
	}
	settings := parseBlastFilterSettings(result.Settings)
	p.blastFilterSettings = settings
	return BlastFilterSettingsResult{Settings: settings}, nil
}

func tuiBlastFilterSettings(settings model.BlastFilterSettings) tui.BlastFilterSettings {
	defaults := model.DefaultBlastFilterSettings()
	if settings == (model.BlastFilterSettings{}) {
		settings = defaults
	}
	return tui.BlastFilterSettings{
		MinIdentityPercent:                        fmt.Sprintf("%.0f", settings.MinIdentityPercent),
		MinAlignQueryCoveragePercent:              fmt.Sprintf("%.0f", settings.MinAlignQueryCoveragePercent),
		MaxEValue:                                 formatScientificSetting(settings.MaxEValue),
		UseTargetCanonicalLengthRatio:             settings.UseTargetCanonicalLengthRatio,
		RequireTargetCanonicalLengthRatio:         settings.RequireTargetCanonicalLengthRatio,
		MinTargetCanonicalLengthPercent:           fmt.Sprintf("%.0f", settings.MinTargetCanonicalLengthPercent),
		MaxTargetCanonicalLengthPercent:           fmt.Sprintf("%.0f", settings.MaxTargetCanonicalLengthPercent),
		UseTargetQueryLengthRatio:                 settings.UseTargetQueryLengthRatio,
		RequireTargetQueryLengthRatio:             settings.RequireTargetQueryLengthRatio,
		MinTargetQueryLengthPercent:               fmt.Sprintf("%.0f", settings.MinTargetQueryLengthPercent),
		MaxTargetQueryLengthPercent:               fmt.Sprintf("%.0f", settings.MaxTargetQueryLengthPercent),
		RequireUniProtAccession:                   settings.RequireUniProtAccession,
		PreferUniProtReviewed:                     settings.PreferUniProtReviewed,
		RejectUniProtFragments:                    settings.RejectUniProtFragments,
		RejectUniProtSequenceCautions:             settings.RejectUniProtSequenceCautions,
		InterProDomainMode:                        settings.InterProDomainMode,
		RequireInterProConservedRegion:            settings.RequireInterProConservedRegion,
		AllowInterProPartial:                      settings.AllowInterProPartial,
		RejectInterProMissing:                     settings.RejectInterProMissing,
		RejectInterProUncertain:                   settings.RejectInterProUncertain,
		MinInterProCoveragePercent:                fmt.Sprintf("%.0f", settings.MinInterProCoveragePercent),
		RequireInterProCoverageWhenUsed:           settings.RequireInterProCoverageWhenUsed,
		AllowStrongBlastFallbackWithoutReferences: settings.AllowStrongBlastFallbackWithoutReferences,
		StrongBlastFallbackMinIdentityPercent:     fmt.Sprintf("%.0f", settings.StrongBlastFallbackMinIdentityPercent),
		StrongBlastFallbackMaxEValue:              formatScientificSetting(settings.StrongBlastFallbackMaxEValue),
		StrongBlastFallbackMinTargetQueryPercent:  fmt.Sprintf("%.0f", settings.StrongBlastFallbackMinTargetQueryPercent),
		StrongBlastFallbackMaxTargetQueryPercent:  fmt.Sprintf("%.0f", settings.StrongBlastFallbackMaxTargetQueryPercent),
		RequireFamilyConsensusForStrongFallback:   settings.RequireFamilyConsensusForStrongFallback,
		StrongFallbackMinFamilyConsensusSupport:   strconv.Itoa(settings.StrongFallbackMinFamilyConsensusSupport),
		StrongFallbackMinFamilyConsensusPercent:   fmt.Sprintf("%.0f", settings.StrongFallbackMinFamilyConsensusPercent),
		UseFamilySemanticAgreement:                settings.UseFamilySemanticAgreement,
		RequireFamilySemanticAgreement:            settings.RequireFamilySemanticAgreement,
		FamilySemanticMinTokenMatches:             strconv.Itoa(settings.FamilySemanticMinTokenMatches),
		FamilySemanticMinAgreementPercent:         fmt.Sprintf("%.0f", settings.FamilySemanticMinAgreementPercent),
		FamilySemanticAllowStrongReferenceBypass:  settings.FamilySemanticAllowStrongReferenceBypass,
		KeepBestIsoformPerTargetGene:              settings.KeepBestIsoformPerTargetGene,
		KeepTopHitsPerQuery:                       settings.KeepTopHitsPerQuery,
		TopHitsPerQuery:                           strconv.Itoa(settings.TopHitsPerQuery),
		RankingTieBreakerOrder:                    settings.RankingTieBreakerOrder,
		PreferHigherFilterScoreWhenRanking:        settings.PreferHigherFilterScoreWhenRanking,
		PreferLowerEValueWhenTies:                 settings.PreferLowerEValueWhenTies,
		PreferHigherIdentityWhenTies:              settings.PreferHigherIdentityWhenTies,
		PreferHigherCoverageWhenTies:              settings.PreferHigherCoverageWhenTies,
		PreferHigherReferenceScoreWhenTies:        settings.PreferHigherReferenceScoreWhenTies,
		PreferHigherBitscoreWhenTies:              settings.PreferHigherBitscoreWhenTies,
		RejectIfAnyHardRuleFails:                  settings.RejectIfAnyHardRuleFails,
		EnableSoftScore:                           settings.EnableSoftScore,
		MinSoftScore:                              strconv.Itoa(settings.MinSoftScore),
		IdentityWeight:                            strconv.Itoa(settings.IdentityWeight),
		CoverageWeight:                            strconv.Itoa(settings.CoverageWeight),
		LengthRatioWeight:                         strconv.Itoa(settings.LengthRatioWeight),
		TargetQueryLengthWeight:                   strconv.Itoa(settings.TargetQueryLengthWeight),
		InterProWeight:                            strconv.Itoa(settings.InterProWeight),
		InterProPartialWeight:                     strconv.Itoa(settings.InterProPartialWeight),
		InterProCoverageWeight:                    strconv.Itoa(settings.InterProCoverageWeight),
		UniProtReviewedWeight:                     strconv.Itoa(settings.UniProtReviewedWeight),
		UniProtAnnotationWeight:                   strconv.Itoa(settings.UniProtAnnotationWeight),
		FamilySemanticAgreementWeight:             strconv.Itoa(settings.FamilySemanticAgreementWeight),
		PenaltySequenceCaution:                    strconv.Itoa(settings.PenaltySequenceCaution),
		PenaltyFragment:                           strconv.Itoa(settings.PenaltyFragment),
		InterProPresentReferenceScore:             strconv.Itoa(settings.InterProPresentReferenceScore),
		InterProPartialReferenceScore:             strconv.Itoa(settings.InterProPartialReferenceScore),
		InterProUncertainReferenceScore:           strconv.Itoa(settings.InterProUncertainReferenceScore),
		InterProMissingReferencePenalty:           strconv.Itoa(settings.InterProMissingReferencePenalty),
		InterProCoverageReferenceDivisor:          strconv.Itoa(settings.InterProCoverageReferenceDivisor),
		UniProtAccessionReferenceScore:            strconv.Itoa(settings.UniProtAccessionReferenceScore),
		UniProtReviewedReferenceScore:             strconv.Itoa(settings.UniProtReviewedReferenceScore),
		UniProtAnnotationReferenceScore:           strconv.Itoa(settings.UniProtAnnotationReferenceScore),
		FamilySemanticReferenceScore:              strconv.Itoa(settings.FamilySemanticReferenceScore),
		FragmentReferencePenaltyMultiplier:        strconv.Itoa(settings.FragmentReferencePenaltyMultiplier),
		SequenceCautionReferencePenaltyMultiplier: strconv.Itoa(settings.SequenceCautionReferencePenaltyMultiplier),
		LengthNearDistancePercent:                 fmt.Sprintf("%.0f", settings.LengthNearDistancePercent),
		LengthNearReferenceScore:                  strconv.Itoa(settings.LengthNearReferenceScore),
		LengthAcceptableDistancePercent:           fmt.Sprintf("%.0f", settings.LengthAcceptableDistancePercent),
		LengthAcceptableReferenceScore:            strconv.Itoa(settings.LengthAcceptableReferenceScore),
		LengthFarDistancePercent:                  fmt.Sprintf("%.0f", settings.LengthFarDistancePercent),
		LengthFarReferencePenalty:                 strconv.Itoa(settings.LengthFarReferencePenalty),
	}
}

func parseBlastFilterSettings(settings tui.BlastFilterSettings) model.BlastFilterSettings {
	defaults := model.DefaultBlastFilterSettings()
	out := model.BlastFilterSettings{
		UseTargetCanonicalLengthRatio:             settings.UseTargetCanonicalLengthRatio,
		RequireTargetCanonicalLengthRatio:         settings.RequireTargetCanonicalLengthRatio,
		UseTargetQueryLengthRatio:                 settings.UseTargetQueryLengthRatio,
		RequireTargetQueryLengthRatio:             settings.RequireTargetQueryLengthRatio,
		RequireUniProtAccession:                   settings.RequireUniProtAccession,
		PreferUniProtReviewed:                     settings.PreferUniProtReviewed,
		RejectUniProtFragments:                    settings.RejectUniProtFragments,
		RejectUniProtSequenceCautions:             settings.RejectUniProtSequenceCautions,
		InterProDomainMode:                        settings.InterProDomainMode,
		RequireInterProConservedRegion:            settings.RequireInterProConservedRegion,
		AllowInterProPartial:                      settings.AllowInterProPartial,
		RejectInterProMissing:                     settings.RejectInterProMissing,
		RejectInterProUncertain:                   settings.RejectInterProUncertain,
		RequireInterProCoverageWhenUsed:           settings.RequireInterProCoverageWhenUsed,
		AllowStrongBlastFallbackWithoutReferences: settings.AllowStrongBlastFallbackWithoutReferences,
		RequireFamilyConsensusForStrongFallback:   settings.RequireFamilyConsensusForStrongFallback,
		UseFamilySemanticAgreement:                settings.UseFamilySemanticAgreement,
		RequireFamilySemanticAgreement:            settings.RequireFamilySemanticAgreement,
		FamilySemanticAllowStrongReferenceBypass:  settings.FamilySemanticAllowStrongReferenceBypass,
		KeepBestIsoformPerTargetGene:              settings.KeepBestIsoformPerTargetGene,
		KeepTopHitsPerQuery:                       settings.KeepTopHitsPerQuery,
		RankingTieBreakerOrder:                    settings.RankingTieBreakerOrder,
		PreferHigherFilterScoreWhenRanking:        settings.PreferHigherFilterScoreWhenRanking,
		PreferLowerEValueWhenTies:                 settings.PreferLowerEValueWhenTies,
		PreferHigherIdentityWhenTies:              settings.PreferHigherIdentityWhenTies,
		PreferHigherCoverageWhenTies:              settings.PreferHigherCoverageWhenTies,
		PreferHigherReferenceScoreWhenTies:        settings.PreferHigherReferenceScoreWhenTies,
		PreferHigherBitscoreWhenTies:              settings.PreferHigherBitscoreWhenTies,
		RejectIfAnyHardRuleFails:                  settings.RejectIfAnyHardRuleFails,
		EnableSoftScore:                           settings.EnableSoftScore,
	}
	out.MinIdentityPercent = parseFloatDefaultAllowZero(settings.MinIdentityPercent, defaults.MinIdentityPercent)
	out.MinAlignQueryCoveragePercent = parseFloatDefaultAllowZero(settings.MinAlignQueryCoveragePercent, defaults.MinAlignQueryCoveragePercent)
	out.MaxEValue = parseFloatDefaultAllowZero(settings.MaxEValue, defaults.MaxEValue)
	out.MinTargetCanonicalLengthPercent = parseFloatDefault(settings.MinTargetCanonicalLengthPercent, defaults.MinTargetCanonicalLengthPercent)
	out.MaxTargetCanonicalLengthPercent = parseFloatDefault(settings.MaxTargetCanonicalLengthPercent, defaults.MaxTargetCanonicalLengthPercent)
	out.MinTargetQueryLengthPercent = parseFloatDefault(settings.MinTargetQueryLengthPercent, defaults.MinTargetQueryLengthPercent)
	out.MaxTargetQueryLengthPercent = parseFloatDefault(settings.MaxTargetQueryLengthPercent, defaults.MaxTargetQueryLengthPercent)
	out.MinInterProCoveragePercent = parseFloatDefaultAllowZero(settings.MinInterProCoveragePercent, defaults.MinInterProCoveragePercent)
	out.StrongBlastFallbackMinIdentityPercent = parseFloatDefault(settings.StrongBlastFallbackMinIdentityPercent, defaults.StrongBlastFallbackMinIdentityPercent)
	out.StrongBlastFallbackMaxEValue = parseFloatDefaultAllowZero(settings.StrongBlastFallbackMaxEValue, defaults.StrongBlastFallbackMaxEValue)
	out.StrongBlastFallbackMinTargetQueryPercent = parseFloatDefault(settings.StrongBlastFallbackMinTargetQueryPercent, defaults.StrongBlastFallbackMinTargetQueryPercent)
	out.StrongBlastFallbackMaxTargetQueryPercent = parseFloatDefault(settings.StrongBlastFallbackMaxTargetQueryPercent, defaults.StrongBlastFallbackMaxTargetQueryPercent)
	out.StrongFallbackMinFamilyConsensusSupport = parseIntDefault(settings.StrongFallbackMinFamilyConsensusSupport, defaults.StrongFallbackMinFamilyConsensusSupport)
	out.StrongFallbackMinFamilyConsensusPercent = parseFloatDefault(settings.StrongFallbackMinFamilyConsensusPercent, defaults.StrongFallbackMinFamilyConsensusPercent)
	out.FamilySemanticMinTokenMatches = parseIntDefault(settings.FamilySemanticMinTokenMatches, defaults.FamilySemanticMinTokenMatches)
	out.FamilySemanticMinAgreementPercent = parseFloatDefaultAllowZero(settings.FamilySemanticMinAgreementPercent, defaults.FamilySemanticMinAgreementPercent)
	if len(parseBlastFilterRankingOrder(out.RankingTieBreakerOrder)) == 0 {
		out.RankingTieBreakerOrder = defaults.RankingTieBreakerOrder
	}
	out.TopHitsPerQuery = parseIntDefault(settings.TopHitsPerQuery, defaults.TopHitsPerQuery)
	out.MinSoftScore = parseIntDefault(settings.MinSoftScore, defaults.MinSoftScore)
	out.IdentityWeight = parseIntDefault(settings.IdentityWeight, defaults.IdentityWeight)
	out.CoverageWeight = parseIntDefault(settings.CoverageWeight, defaults.CoverageWeight)
	out.LengthRatioWeight = parseIntDefault(settings.LengthRatioWeight, defaults.LengthRatioWeight)
	out.TargetQueryLengthWeight = parseIntDefault(settings.TargetQueryLengthWeight, defaults.TargetQueryLengthWeight)
	out.InterProWeight = parseIntDefault(settings.InterProWeight, defaults.InterProWeight)
	out.InterProPartialWeight = parseIntDefault(settings.InterProPartialWeight, defaults.InterProPartialWeight)
	out.InterProCoverageWeight = parseIntDefault(settings.InterProCoverageWeight, defaults.InterProCoverageWeight)
	out.UniProtReviewedWeight = parseIntDefault(settings.UniProtReviewedWeight, defaults.UniProtReviewedWeight)
	out.UniProtAnnotationWeight = parseIntDefault(settings.UniProtAnnotationWeight, defaults.UniProtAnnotationWeight)
	out.FamilySemanticAgreementWeight = parseIntDefault(settings.FamilySemanticAgreementWeight, defaults.FamilySemanticAgreementWeight)
	out.PenaltySequenceCaution = parseIntDefault(settings.PenaltySequenceCaution, defaults.PenaltySequenceCaution)
	out.PenaltyFragment = parseIntDefault(settings.PenaltyFragment, defaults.PenaltyFragment)
	out.InterProPresentReferenceScore = parseIntDefault(settings.InterProPresentReferenceScore, defaults.InterProPresentReferenceScore)
	out.InterProPartialReferenceScore = parseIntDefault(settings.InterProPartialReferenceScore, defaults.InterProPartialReferenceScore)
	out.InterProUncertainReferenceScore = parseIntDefault(settings.InterProUncertainReferenceScore, defaults.InterProUncertainReferenceScore)
	out.InterProMissingReferencePenalty = parseIntDefault(settings.InterProMissingReferencePenalty, defaults.InterProMissingReferencePenalty)
	out.InterProCoverageReferenceDivisor = parseIntDefault(settings.InterProCoverageReferenceDivisor, defaults.InterProCoverageReferenceDivisor)
	out.UniProtAccessionReferenceScore = parseIntDefault(settings.UniProtAccessionReferenceScore, defaults.UniProtAccessionReferenceScore)
	out.UniProtReviewedReferenceScore = parseIntDefault(settings.UniProtReviewedReferenceScore, defaults.UniProtReviewedReferenceScore)
	out.UniProtAnnotationReferenceScore = parseIntDefault(settings.UniProtAnnotationReferenceScore, defaults.UniProtAnnotationReferenceScore)
	out.FamilySemanticReferenceScore = parseIntDefault(settings.FamilySemanticReferenceScore, defaults.FamilySemanticReferenceScore)
	out.FragmentReferencePenaltyMultiplier = parseIntDefault(settings.FragmentReferencePenaltyMultiplier, defaults.FragmentReferencePenaltyMultiplier)
	out.SequenceCautionReferencePenaltyMultiplier = parseIntDefault(settings.SequenceCautionReferencePenaltyMultiplier, defaults.SequenceCautionReferencePenaltyMultiplier)
	out.LengthNearDistancePercent = parseFloatDefault(settings.LengthNearDistancePercent, defaults.LengthNearDistancePercent)
	out.LengthNearReferenceScore = parseIntDefault(settings.LengthNearReferenceScore, defaults.LengthNearReferenceScore)
	out.LengthAcceptableDistancePercent = parseFloatDefault(settings.LengthAcceptableDistancePercent, defaults.LengthAcceptableDistancePercent)
	out.LengthAcceptableReferenceScore = parseIntDefault(settings.LengthAcceptableReferenceScore, defaults.LengthAcceptableReferenceScore)
	out.LengthFarDistancePercent = parseFloatDefault(settings.LengthFarDistancePercent, defaults.LengthFarDistancePercent)
	out.LengthFarReferencePenalty = parseIntDefault(settings.LengthFarReferencePenalty, defaults.LengthFarReferencePenalty)
	if out.MaxTargetCanonicalLengthPercent < out.MinTargetCanonicalLengthPercent {
		out.MaxTargetCanonicalLengthPercent = out.MinTargetCanonicalLengthPercent
	}
	if out.MaxTargetQueryLengthPercent < out.MinTargetQueryLengthPercent {
		out.MaxTargetQueryLengthPercent = out.MinTargetQueryLengthPercent
	}
	return out
}

func (p *Prompter) blastFilterSuggestion(request BlastFilterRequest) (BlastFilterSuggestion, error) {
	if BlastFilterSuggest != nil {
		return BlastFilterSuggest(request)
	}
	return defaultBlastFilterSuggestion(request), nil
}

func (p *Prompter) blastFilterSuggestionWithProgress(request BlastFilterRequest) (BlastFilterSuggestion, error) {
	total := len(request.Rows)
	if len(request.RowsByRun) > 0 {
		total = 0
		for _, rows := range request.RowsByRun {
			total += len(rows)
		}
	}
	if total <= 0 {
		return p.blastFilterSuggestion(request)
	}
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        p.blastTUIPath("BLAST input", "BLAST results", "Filter"),
		Title:       "Applying BLAST filter",
		Description: "Applying the current filter settings and rebuilding row checkbox suggestions.",
		Initial:     "Preparing filter evaluation...",
		Total:       total,
	}, func(ctx context.Context, update func(current int, message string)) (BlastFilterSuggestion, error) {
		update(0, "Evaluating BLAST result rows...")
		suggestion, err := p.blastFilterSuggestion(request)
		if err != nil {
			return BlastFilterSuggestion{}, err
		}
		select {
		case <-ctx.Done():
			return BlastFilterSuggestion{}, ctx.Err()
		default:
		}
		update(total, "Filter suggestions are ready.")
		return suggestion, nil
	})
}

func (p *Prompter) clearBlastFilterWithProgress(rowCount int) (blastFilterClearResult, error) {
	if rowCount <= 0 {
		return blastFilterClearResult{}, nil
	}
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        p.blastTUIPath("BLAST input", "BLAST results", "Filter"),
		Title:       "Clearing BLAST filter",
		Description: "Removing filter suggestion marks and reselecting all rows.",
		Initial:     "Clearing filter marks...",
		Total:       rowCount,
	}, func(ctx context.Context, update func(current int, message string)) (blastFilterClearResult, error) {
		selected := make([]bool, rowCount)
		flags := make([]bool, rowCount)
		for i := range selected {
			select {
			case <-ctx.Done():
				return blastFilterClearResult{}, ctx.Err()
			default:
			}
			selected[i] = true
			if i == len(selected)-1 || (i+1)%500 == 0 {
				update(i+1, fmt.Sprintf("Cleared filter marks... %d/%d", i+1, rowCount))
			}
		}
		_ = flags
		update(rowCount, "Filter marks cleared.")
		return blastFilterClearResult{Selected: selected, Flags: flags}, nil
	})
}

func (p *Prompter) clearBlastRunFiltersWithProgress(rowCounts []int) (blastFilterClearResult, error) {
	total := 0
	for _, count := range rowCounts {
		if count > 0 {
			total += count
		}
	}
	if total <= 0 {
		return blastFilterClearResult{}, nil
	}
	return tui.RunProgressTaskValueContext(tui.TaskPage{
		Path:        p.blastTUIPath("BLAST input", "BLAST results", "Filter"),
		Title:       "Clearing BLAST filter",
		Description: "Removing filter suggestion marks and reselecting all rows for every BLAST query.",
		Initial:     "Clearing filter marks...",
		Total:       total,
	}, func(ctx context.Context, update func(current int, message string)) (blastFilterClearResult, error) {
		selectedByRun := make([][]bool, len(rowCounts))
		flagsByRun := make([][]bool, len(rowCounts))
		done := 0
		for runIndex, count := range rowCounts {
			if count < 0 {
				count = 0
			}
			selectedByRun[runIndex] = make([]bool, count)
			flagsByRun[runIndex] = make([]bool, count)
			for rowIndex := 0; rowIndex < count; rowIndex++ {
				select {
				case <-ctx.Done():
					return blastFilterClearResult{}, ctx.Err()
				default:
				}
				selectedByRun[runIndex][rowIndex] = true
				done++
				if done == total || done%500 == 0 {
					update(done, fmt.Sprintf("Cleared filter marks... %d/%d", done, total))
				}
			}
		}
		update(total, "Filter marks cleared.")
		return blastFilterClearResult{SelectedByRun: selectedByRun, FlagsByRun: flagsByRun}, nil
	})
}

func blastFilterTaskCancelled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, tui.ErrTaskCancelled)
}

func defaultBlastFilterSuggestion(request BlastFilterRequest) BlastFilterSuggestion {
	settings := normalizeBlastFilterSettings(request.Settings)
	if len(request.RowsByRun) > 0 {
		selectedByRun := make([][]bool, len(request.RowsByRun))
		flagsByRun := make([][]bool, len(request.RowsByRun))
		for runIndex, rows := range request.RowsByRun {
			selected := []bool(nil)
			if runIndex < len(request.SelectedByRun) {
				selected = request.SelectedByRun[runIndex]
			}
			suggestion := blastFilterSuggestRows(rows, selected, settings)
			selectedByRun[runIndex] = suggestion.Selected
			flagsByRun[runIndex] = suggestion.Flags
		}
		out := BlastFilterSuggestion{
			SelectedByRun: selectedByRun,
			FlagsByRun:    flagsByRun,
			Settings:      settings,
		}
		if request.CurrentRun >= 0 && request.CurrentRun < len(selectedByRun) {
			out.Selected = append([]bool(nil), selectedByRun[request.CurrentRun]...)
			out.Flags = append([]bool(nil), flagsByRun[request.CurrentRun]...)
		}
		return out
	}
	out := blastFilterSuggestRows(request.Rows, request.Selected, settings)
	out.Settings = settings
	return out
}

func DefaultBlastFilterSuggestion(request BlastFilterRequest) BlastFilterSuggestion {
	return defaultBlastFilterSuggestion(request)
}

func blastFilterSuggestRows(rows []model.BlastResultRow, selected []bool, settings model.BlastFilterSettings) BlastFilterSuggestion {
	outSelected := normalizePromptSelection(nil, len(rows), true)
	flags := make([]bool, len(rows))
	evaluations := make([]blastFilterEvaluation, len(rows))
	for i, row := range rows {
		evaluations[i] = evaluateBlastFilterRow(row, settings)
		flags[i] = evaluations[i].Reject
		if flags[i] {
			outSelected[i] = false
		}
	}
	if settings.KeepBestIsoformPerTargetGene {
		applyBestIsoformLimit(rows, evaluations, outSelected, flags, settings)
	}
	if settings.KeepTopHitsPerQuery && settings.TopHitsPerQuery > 0 {
		applyTopHitLimit(rows, evaluations, outSelected, flags, settings)
	}
	return BlastFilterSuggestion{Selected: outSelected, Flags: flags, Settings: settings}
}

func normalizePromptSelection(selected []bool, length int, defaultValue bool) []bool {
	out := make([]bool, length)
	for i := range out {
		out[i] = defaultValue
	}
	if len(selected) == length {
		copy(out, selected)
	}
	return out
}

func evaluateBlastFilterRow(row model.BlastResultRow, settings model.BlastFilterSettings) blastFilterEvaluation {
	evaluation := blastFilterEvaluation{EValue: parseScientificFloat(row.EValue, 1e300)}
	hardFailed := false
	identity := row.PercentIdentity
	if settings.MinIdentityPercent <= 0 {
		// The default filter is calibrated around length and conserved-region evidence.
		// Identity can still be enabled as an extra strict hard rule by setting a value.
	} else if identity >= settings.MinIdentityPercent {
		evaluation.Score += settings.IdentityWeight
	} else {
		hardFailed = true
	}
	coverage := blastRowQueryCoverage(row)
	if settings.MinAlignQueryCoveragePercent <= 0 {
		// Query coverage can be enabled as an extra strict hard rule by setting a value.
	} else if coverage >= settings.MinAlignQueryCoveragePercent {
		evaluation.Score += settings.CoverageWeight
	} else {
		hardFailed = true
	}
	if settings.MaxEValue > 0 && evaluation.EValue > settings.MaxEValue {
		hardFailed = true
	}
	if settings.UseTargetCanonicalLengthRatio {
		ratio := parseFirstFloat(row.TargetUniProtCanonicalLengthPercent)
		if ratio > 0 {
			if ratio >= settings.MinTargetCanonicalLengthPercent && ratio <= settings.MaxTargetCanonicalLengthPercent {
				evaluation.Score += settings.LengthRatioWeight
			} else {
				hardFailed = true
			}
		} else if settings.RequireTargetCanonicalLengthRatio {
			hardFailed = true
		}
	}
	if settings.UseTargetQueryLengthRatio {
		targetQueryRatio := blastTargetQueryLengthRatio(row)
		if targetQueryRatio > 0 {
			if targetQueryRatio >= settings.MinTargetQueryLengthPercent && targetQueryRatio <= settings.MaxTargetQueryLengthPercent {
				evaluation.Score += settings.TargetQueryLengthWeight
			} else {
				hardFailed = true
			}
		} else if settings.RequireTargetQueryLengthRatio {
			hardFailed = true
		}
	}
	if settings.RequireUniProtAccession && strings.TrimSpace(row.UniProtAccession) == "" {
		hardFailed = true
	}
	if settings.PreferUniProtReviewed && strings.EqualFold(strings.TrimSpace(row.UniProtReviewed), "reviewed") {
		evaluation.Score += settings.UniProtReviewedWeight
	}
	if hasUniProtAnnotation(row) {
		evaluation.Score += settings.UniProtAnnotationWeight
	}
	if settings.UseFamilySemanticAgreement && blastRowHasSemanticTokens(row) && row.FamilySemanticAnnotationMatchCount > 0 {
		evaluation.Score += settings.FamilySemanticAgreementWeight
	}
	if settings.UseFamilySemanticAgreement && settings.RequireFamilySemanticAgreement && blastRowHasSemanticTokens(row) && blastRowHasSemanticReferenceSurface(row) && !blastRowHasSemanticAgreement(row, settings) {
		if !(settings.FamilySemanticAllowStrongReferenceBypass && !blastRowHasSemanticReferenceSurface(row) && blastFilterReferenceScore(row, settings) >= strongReferenceBypassScore(settings)) {
			hardFailed = true
		}
	}
	if settings.RejectUniProtFragments && isTruthyAnnotation(row.UniProtFragment) {
		hardFailed = true
		evaluation.Score -= settings.PenaltyFragment
	}
	if settings.RejectUniProtSequenceCautions && strings.TrimSpace(row.UniProtSequenceCaution) != "" {
		hardFailed = true
		evaluation.Score -= settings.PenaltySequenceCaution
	}
	status := strings.ToLower(strings.TrimSpace(row.InterProConservedRegionStatus))
	switch strings.ToLower(strings.TrimSpace(settings.InterProDomainMode)) {
	case "off":
	case "conserved_region":
		switch status {
		case "present":
			evaluation.Score += settings.InterProWeight
		case "partial":
			if settings.AllowInterProPartial {
				evaluation.Score += settings.InterProPartialWeight
			}
			if settings.RequireInterProConservedRegion && !settings.AllowInterProPartial {
				hardFailed = true
			}
		case "missing":
			if settings.RejectInterProMissing || settings.RequireInterProConservedRegion {
				hardFailed = true
			}
		case "uncertain":
			if settings.RejectInterProUncertain || settings.RequireInterProConservedRegion {
				hardFailed = true
			}
		case "":
			if settings.RequireInterProConservedRegion || settings.RejectInterProMissing {
				hardFailed = true
			}
		}
	case "family_consensus_domain", "any_domain":
		if blastRowHasAnyInterProDomain(row) {
			evaluation.Score += settings.InterProWeight
		} else {
			hardFailed = true
		}
	default:
		if blastRowHasAnyInterProDomain(row) {
			evaluation.Score += settings.InterProWeight
		}
	}
	if settings.MinInterProCoveragePercent > 0 {
		coverage := parseFirstFloat(row.InterProCoveragePercent)
		if coverage > 0 {
			if coverage >= settings.MinInterProCoveragePercent {
				evaluation.Score += settings.InterProCoverageWeight
			} else {
				hardFailed = true
			}
		} else if settings.RequireInterProCoverageWhenUsed {
			hardFailed = true
		}
	}
	if hardFailed && settings.AllowStrongBlastFallbackWithoutReferences && blastRowMissingReferenceAnchors(row) && blastRowMeetsStrongFallback(row, evaluation.EValue, settings) {
		hardFailed = false
	}
	if settings.RejectIfAnyHardRuleFails && hardFailed {
		evaluation.Reject = true
	}
	if settings.EnableSoftScore && evaluation.Score < settings.MinSoftScore {
		evaluation.Reject = true
	}
	return evaluation
}

func blastRowMissingReferenceAnchors(row model.BlastResultRow) bool {
	return strings.TrimSpace(row.UniProtAccession) == "" && strings.TrimSpace(row.InterProConservedRegionStatus) == ""
}

func blastRowMeetsStrongFallback(row model.BlastResultRow, eValue float64, settings model.BlastFilterSettings) bool {
	if row.PercentIdentity < settings.StrongBlastFallbackMinIdentityPercent {
		return false
	}
	if settings.StrongBlastFallbackMaxEValue > 0 && eValue > settings.StrongBlastFallbackMaxEValue {
		return false
	}
	ratio := blastTargetQueryLengthRatio(row)
	if ratio <= 0 {
		return false
	}
	if ratio < settings.StrongBlastFallbackMinTargetQueryPercent || ratio > settings.StrongBlastFallbackMaxTargetQueryPercent {
		return false
	}
	if settings.RequireFamilyConsensusForStrongFallback && !blastRowMeetsStrongFallbackConsensus(row, settings) {
		return false
	}
	return true
}

func blastRowMeetsStrongFallbackConsensus(row model.BlastResultRow, settings model.BlastFilterSettings) bool {
	support := row.FamilyConsensusSupport
	if support < settings.StrongFallbackMinFamilyConsensusSupport {
		return false
	}
	if settings.StrongFallbackMinFamilyConsensusPercent <= 0 {
		return true
	}
	coverage := parseFirstFloat(row.FamilyConsensusCoveragePercent)
	return coverage >= settings.StrongFallbackMinFamilyConsensusPercent
}

func blastTargetQueryLengthRatio(row model.BlastResultRow) float64 {
	if row.TargetLength <= 0 || row.QueryLength <= 0 {
		return 0
	}
	return float64(row.TargetLength) / float64(row.QueryLength) * 100
}

func blastRowHasAnyInterProDomain(row model.BlastResultRow) bool {
	for _, value := range []string{
		row.InterProConservedRegionStatus,
		row.InterProAccessions,
		row.InterProSignatureAccessions,
		row.InterProPfamAccessions,
		row.PfamDomain,
		row.UniProtPfam,
		row.UniProtInterPro,
		row.UniProtDomain,
	} {
		if strings.TrimSpace(value) != "" && !strings.EqualFold(strings.TrimSpace(value), "missing") {
			return true
		}
	}
	return false
}

func applyTopHitLimit(rows []model.BlastResultRow, evaluations []blastFilterEvaluation, selected []bool, flags []bool, settings model.BlastFilterSettings) {
	groups := make(map[string][]int, len(rows))
	for i, row := range rows {
		key := blastFilterQueryKey(row)
		groups[key] = append(groups[key], i)
	}
	for _, indexes := range groups {
		sortBlastFilterIndexes(rows, evaluations, indexes, settings)
		kept := 0
		for _, index := range indexes {
			if flags[index] {
				continue
			}
			kept++
			if kept > settings.TopHitsPerQuery {
				flags[index] = true
				selected[index] = false
			}
		}
	}
}

func applyBestIsoformLimit(rows []model.BlastResultRow, evaluations []blastFilterEvaluation, selected []bool, flags []bool, settings model.BlastFilterSettings) {
	groups := make(map[string][]int, len(rows))
	for i, row := range rows {
		key := blastFilterTargetGeneKey(row)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], i)
	}
	for _, indexes := range groups {
		if len(indexes) <= 1 {
			continue
		}
		sortBlastFilterIndexes(rows, evaluations, indexes, settings)
		kept := false
		for _, index := range indexes {
			if flags[index] {
				continue
			}
			if !kept {
				kept = true
				continue
			}
			flags[index] = true
			selected[index] = false
		}
	}
}

func sortBlastFilterIndexes(rows []model.BlastResultRow, evaluations []blastFilterEvaluation, indexes []int, settings model.BlastFilterSettings) {
	sort.SliceStable(indexes, func(i, j int) bool {
		return blastFilterIndexLess(rows, evaluations, indexes[i], indexes[j], settings)
	})
}

func blastFilterIndexLess(rows []model.BlastResultRow, evaluations []blastFilterEvaluation, leftIndex int, rightIndex int, settings model.BlastFilterSettings) bool {
	left := evaluations[leftIndex]
	right := evaluations[rightIndex]
	if left.Reject != right.Reject {
		return !left.Reject
	}
	for _, field := range blastFilterRankingOrder(settings) {
		switch field {
		case "score":
			if settings.PreferHigherFilterScoreWhenRanking && left.Score != right.Score {
				return left.Score > right.Score
			}
		case "identity":
			if settings.PreferHigherIdentityWhenTies && rows[leftIndex].PercentIdentity != rows[rightIndex].PercentIdentity {
				return rows[leftIndex].PercentIdentity > rows[rightIndex].PercentIdentity
			}
		case "coverage":
			leftCoverage := blastRowQueryCoverage(rows[leftIndex])
			rightCoverage := blastRowQueryCoverage(rows[rightIndex])
			if settings.PreferHigherCoverageWhenTies && leftCoverage != rightCoverage {
				return leftCoverage > rightCoverage
			}
		case "reference":
			leftEvidence := blastFilterReferenceScore(rows[leftIndex], settings)
			rightEvidence := blastFilterReferenceScore(rows[rightIndex], settings)
			if settings.PreferHigherReferenceScoreWhenTies && leftEvidence != rightEvidence {
				return leftEvidence > rightEvidence
			}
		case "evalue":
			if settings.PreferLowerEValueWhenTies && left.EValue != right.EValue {
				return left.EValue < right.EValue
			}
		case "bitscore":
			if settings.PreferHigherBitscoreWhenTies && rows[leftIndex].Bitscore != rows[rightIndex].Bitscore {
				return rows[leftIndex].Bitscore > rows[rightIndex].Bitscore
			}
		}
	}
	return leftIndex < rightIndex
}

func blastFilterRankingOrder(settings model.BlastFilterSettings) []string {
	order := parseBlastFilterRankingOrder(settings.RankingTieBreakerOrder)
	if len(order) == 0 {
		order = parseBlastFilterRankingOrder(model.DefaultBlastFilterSettings().RankingTieBreakerOrder)
	}
	return order
}

func parseBlastFilterRankingOrder(value string) []string {
	known := map[string]bool{
		"score":     true,
		"identity":  true,
		"coverage":  true,
		"reference": true,
		"evalue":    true,
		"bitscore":  true,
	}
	seen := make(map[string]bool, len(known))
	out := make([]string, 0, len(known))
	for _, part := range strings.Split(value, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		part = strings.ReplaceAll(part, "-", "")
		part = strings.ReplaceAll(part, "_", "")
		switch part {
		case "filter", "filterscore", "softscore", "hardscore":
			part = "score"
		case "querycoverage", "aligncoverage", "alignquerycoverage":
			part = "coverage"
		case "ref", "referencescore", "externalevidence", "evidence":
			part = "reference"
		case "eval", "e-value", "e_value":
			part = "evalue"
		case "bit", "bit-score", "bit_score":
			part = "bitscore"
		}
		if !known[part] || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}

func blastFilterReferenceScore(row model.BlastResultRow, settings model.BlastFilterSettings) int {
	score := 0
	switch strings.ToLower(strings.TrimSpace(row.InterProConservedRegionStatus)) {
	case "present":
		score += settings.InterProPresentReferenceScore
	case "partial":
		score += settings.InterProPartialReferenceScore
	case "uncertain":
		score += settings.InterProUncertainReferenceScore
	case "missing":
		score -= settings.InterProMissingReferencePenalty
	}
	if coverage := parseFirstFloat(row.InterProCoveragePercent); coverage > 0 && settings.InterProCoverageReferenceDivisor > 0 {
		score += int(coverage / float64(settings.InterProCoverageReferenceDivisor))
	}
	if strings.TrimSpace(row.UniProtAccession) != "" {
		score += settings.UniProtAccessionReferenceScore
	}
	if strings.EqualFold(strings.TrimSpace(row.UniProtReviewed), "reviewed") {
		score += settings.UniProtReviewedReferenceScore
	}
	if hasUniProtAnnotation(row) {
		score += settings.UniProtAnnotationReferenceScore
	}
	if settings.UseFamilySemanticAgreement && blastRowHasSemanticTokens(row) && blastRowHasSemanticAgreement(row, settings) {
		score += settings.FamilySemanticReferenceScore
	}
	if isTruthyAnnotation(row.UniProtFragment) {
		score -= settings.PenaltyFragment * settings.FragmentReferencePenaltyMultiplier
	}
	if strings.TrimSpace(row.UniProtSequenceCaution) != "" {
		score -= settings.PenaltySequenceCaution * settings.SequenceCautionReferencePenaltyMultiplier
	}
	if ratio := parseFirstFloat(row.TargetUniProtCanonicalLengthPercent); ratio > 0 {
		distance := ratio - 100
		if distance < 0 {
			distance = -distance
		}
		switch {
		case distance <= settings.LengthNearDistancePercent:
			score += settings.LengthNearReferenceScore
		case distance <= settings.LengthAcceptableDistancePercent:
			score += settings.LengthAcceptableReferenceScore
		case distance >= settings.LengthFarDistancePercent:
			score -= settings.LengthFarReferencePenalty
		}
	}
	return score
}

func blastRowHasSemanticAgreement(row model.BlastResultRow, settings model.BlastFilterSettings) bool {
	if !blastRowHasSemanticTokens(row) {
		return true
	}
	if row.FamilySemanticAnnotationMatchCount < settings.FamilySemanticMinTokenMatches {
		return false
	}
	if settings.FamilySemanticMinAgreementPercent <= 0 {
		return true
	}
	return parseFirstFloat(row.FamilySemanticAgreementPercent) >= settings.FamilySemanticMinAgreementPercent
}

func blastRowHasSemanticTokens(row model.BlastResultRow) bool {
	return strings.TrimSpace(row.FamilySemanticTokens) != "" || strings.TrimSpace(row.FamilySemanticAliasTokens) != ""
}

func blastRowHasSemanticReferenceSurface(row model.BlastResultRow) bool {
	for _, value := range []string{
		row.UniProtProteinName,
		row.UniProtEntryName,
		row.UniProtGeneNames,
		row.UniProtKeywords,
		row.UniProtFunction,
		row.UniProtCatalyticActivity,
		row.UniProtPathway,
		row.UniProtDomain,
		row.UniProtInterPro,
		row.PfamDomain,
		row.InterProEntryName,
	} {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func strongReferenceBypassScore(settings model.BlastFilterSettings) int {
	score := settings.InterProPresentReferenceScore + settings.UniProtAccessionReferenceScore + settings.UniProtAnnotationReferenceScore
	if score <= 0 {
		return 100
	}
	return score
}

func blastFilterTargetGeneKey(row model.BlastResultRow) string {
	for _, value := range []string{row.Protein, row.SubjectID, row.TranscriptID, row.SequenceID, row.GeneReportURL} {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		value = strings.TrimSuffix(value, "/")
		if slash := strings.LastIndex(value, "/"); slash >= 0 && slash < len(value)-1 {
			value = value[slash+1:]
		}
		value = strings.TrimSpace(value)
		value = regexp.MustCompile(`(?i)_t\d+$`).ReplaceAllString(value, "")
		value = regexp.MustCompile(`(?i)[._-]t\d+$`).ReplaceAllString(value, "")
		value = regexp.MustCompile(`(?i)\.\d+$`).ReplaceAllString(value, "")
		if value != "" {
			return value
		}
	}
	return ""
}

func blastFilterQueryKey(row model.BlastResultRow) string {
	for _, value := range []string{row.QueryID, row.LabelName} {
		value = strings.TrimSpace(value)
		if value != "" {
			return strings.ToLower(value)
		}
	}
	return "query"
}

func blastRowQueryCoverage(row model.BlastResultRow) float64 {
	if row.AlignQueryLengthPercent > 0 {
		return row.AlignQueryLengthPercent
	}
	if row.QueryLength > 0 && row.AlignLength > 0 {
		return float64(row.AlignLength) / float64(row.QueryLength) * 100
	}
	if row.QueryLength > 0 {
		if span := coordinateSpanPrompt(row.QueryFrom, row.QueryTo); span > 0 {
			return float64(span) / float64(row.QueryLength) * 100
		}
	}
	return 0
}

func hasUniProtAnnotation(row model.BlastResultRow) bool {
	for _, value := range []string{row.UniProtProteinName, row.UniProtGeneNames, row.UniProtKeywords, row.UniProtEC, row.UniProtGO, row.UniProtFunction, row.UniProtCatalyticActivity, row.UniProtPathway} {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func isTruthyAnnotation(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "false", "no", "0", "none", "not fragment":
		return false
	default:
		return true
	}
}

func parseFirstFloat(value string) float64 {
	return parseScientificFloat(value, 0)
}

func parseScientificFloat(value string, fallback float64) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	match := numericValuePattern.FindString(value)
	if match == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(match, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func coordinateSpanPrompt(from int, to int) int {
	if from <= 0 || to <= 0 {
		return 0
	}
	if from > to {
		from, to = to, from
	}
	return to - from + 1
}

func normalizeBlastFilterSettings(settings model.BlastFilterSettings) model.BlastFilterSettings {
	defaults := model.DefaultBlastFilterSettings()
	if settings == (model.BlastFilterSettings{}) {
		settings = defaults
	}
	if settings.MinIdentityPercent < 0 {
		settings.MinIdentityPercent = defaults.MinIdentityPercent
	}
	if settings.MinAlignQueryCoveragePercent < 0 {
		settings.MinAlignQueryCoveragePercent = defaults.MinAlignQueryCoveragePercent
	}
	if settings.MaxEValue < 0 {
		settings.MaxEValue = defaults.MaxEValue
	}
	if settings.MinTargetCanonicalLengthPercent <= 0 {
		settings.MinTargetCanonicalLengthPercent = defaults.MinTargetCanonicalLengthPercent
	}
	if settings.MaxTargetCanonicalLengthPercent <= 0 {
		settings.MaxTargetCanonicalLengthPercent = defaults.MaxTargetCanonicalLengthPercent
	}
	if settings.MaxTargetCanonicalLengthPercent < settings.MinTargetCanonicalLengthPercent {
		settings.MaxTargetCanonicalLengthPercent = settings.MinTargetCanonicalLengthPercent
	}
	if !settings.UseTargetCanonicalLengthRatio {
		settings.RequireTargetCanonicalLengthRatio = false
	}
	if settings.MinTargetQueryLengthPercent <= 0 {
		settings.MinTargetQueryLengthPercent = defaults.MinTargetQueryLengthPercent
	}
	if settings.MaxTargetQueryLengthPercent <= 0 {
		settings.MaxTargetQueryLengthPercent = defaults.MaxTargetQueryLengthPercent
	}
	if settings.MaxTargetQueryLengthPercent < settings.MinTargetQueryLengthPercent {
		settings.MaxTargetQueryLengthPercent = settings.MinTargetQueryLengthPercent
	}
	if !settings.UseTargetQueryLengthRatio {
		settings.RequireTargetQueryLengthRatio = false
	}
	if strings.TrimSpace(settings.InterProDomainMode) == "" {
		settings.InterProDomainMode = defaults.InterProDomainMode
	}
	if settings.TopHitsPerQuery <= 0 {
		settings.TopHitsPerQuery = defaults.TopHitsPerQuery
	}
	if settings.FamilySemanticMinTokenMatches <= 0 {
		settings.FamilySemanticMinTokenMatches = defaults.FamilySemanticMinTokenMatches
	}
	if settings.MinSoftScore <= 0 {
		settings.MinSoftScore = defaults.MinSoftScore
	}
	if len(parseBlastFilterRankingOrder(settings.RankingTieBreakerOrder)) == 0 {
		settings.RankingTieBreakerOrder = defaults.RankingTieBreakerOrder
	}
	if settings.IdentityWeight <= 0 {
		settings.IdentityWeight = defaults.IdentityWeight
	}
	if settings.CoverageWeight <= 0 {
		settings.CoverageWeight = defaults.CoverageWeight
	}
	if settings.LengthRatioWeight <= 0 {
		settings.LengthRatioWeight = defaults.LengthRatioWeight
	}
	if settings.TargetQueryLengthWeight <= 0 {
		settings.TargetQueryLengthWeight = defaults.TargetQueryLengthWeight
	}
	if settings.InterProWeight <= 0 {
		settings.InterProWeight = defaults.InterProWeight
	}
	if settings.InterProPartialWeight <= 0 {
		settings.InterProPartialWeight = defaults.InterProPartialWeight
	}
	if settings.InterProCoverageWeight <= 0 {
		settings.InterProCoverageWeight = defaults.InterProCoverageWeight
	}
	if settings.UniProtReviewedWeight <= 0 {
		settings.UniProtReviewedWeight = defaults.UniProtReviewedWeight
	}
	if settings.UniProtAnnotationWeight <= 0 {
		settings.UniProtAnnotationWeight = defaults.UniProtAnnotationWeight
	}
	if settings.FamilySemanticAgreementWeight <= 0 {
		settings.FamilySemanticAgreementWeight = defaults.FamilySemanticAgreementWeight
	}
	if settings.PenaltySequenceCaution <= 0 {
		settings.PenaltySequenceCaution = defaults.PenaltySequenceCaution
	}
	if settings.PenaltyFragment <= 0 {
		settings.PenaltyFragment = defaults.PenaltyFragment
	}
	if settings.InterProPresentReferenceScore <= 0 {
		settings.InterProPresentReferenceScore = defaults.InterProPresentReferenceScore
	}
	if settings.InterProPartialReferenceScore <= 0 {
		settings.InterProPartialReferenceScore = defaults.InterProPartialReferenceScore
	}
	if settings.InterProUncertainReferenceScore <= 0 {
		settings.InterProUncertainReferenceScore = defaults.InterProUncertainReferenceScore
	}
	if settings.InterProMissingReferencePenalty <= 0 {
		settings.InterProMissingReferencePenalty = defaults.InterProMissingReferencePenalty
	}
	if settings.InterProCoverageReferenceDivisor <= 0 {
		settings.InterProCoverageReferenceDivisor = defaults.InterProCoverageReferenceDivisor
	}
	if settings.UniProtAccessionReferenceScore <= 0 {
		settings.UniProtAccessionReferenceScore = defaults.UniProtAccessionReferenceScore
	}
	if settings.UniProtReviewedReferenceScore <= 0 {
		settings.UniProtReviewedReferenceScore = defaults.UniProtReviewedReferenceScore
	}
	if settings.UniProtAnnotationReferenceScore <= 0 {
		settings.UniProtAnnotationReferenceScore = defaults.UniProtAnnotationReferenceScore
	}
	if settings.FamilySemanticReferenceScore <= 0 {
		settings.FamilySemanticReferenceScore = defaults.FamilySemanticReferenceScore
	}
	if settings.FragmentReferencePenaltyMultiplier <= 0 {
		settings.FragmentReferencePenaltyMultiplier = defaults.FragmentReferencePenaltyMultiplier
	}
	if settings.SequenceCautionReferencePenaltyMultiplier <= 0 {
		settings.SequenceCautionReferencePenaltyMultiplier = defaults.SequenceCautionReferencePenaltyMultiplier
	}
	if settings.LengthNearDistancePercent <= 0 {
		settings.LengthNearDistancePercent = defaults.LengthNearDistancePercent
	}
	if settings.LengthNearReferenceScore <= 0 {
		settings.LengthNearReferenceScore = defaults.LengthNearReferenceScore
	}
	if settings.LengthAcceptableDistancePercent <= 0 {
		settings.LengthAcceptableDistancePercent = defaults.LengthAcceptableDistancePercent
	}
	if settings.LengthAcceptableReferenceScore <= 0 {
		settings.LengthAcceptableReferenceScore = defaults.LengthAcceptableReferenceScore
	}
	if settings.LengthFarDistancePercent <= 0 {
		settings.LengthFarDistancePercent = defaults.LengthFarDistancePercent
	}
	if settings.LengthFarReferencePenalty <= 0 {
		settings.LengthFarReferencePenalty = defaults.LengthFarReferencePenalty
	}
	if settings.LengthAcceptableDistancePercent < settings.LengthNearDistancePercent {
		settings.LengthAcceptableDistancePercent = settings.LengthNearDistancePercent
	}
	if settings.LengthFarDistancePercent < settings.LengthAcceptableDistancePercent {
		settings.LengthFarDistancePercent = settings.LengthAcceptableDistancePercent
	}
	return settings
}

func blastRowsHaveAllExternalReferences(rows []model.BlastResultRow) bool {
	if len(rows) == 0 {
		return false
	}
	for _, row := range rows {
		if !row.UniProtReferenceEnabled || !row.InterProReferenceEnabled {
			return false
		}
	}
	return true
}

func blastRunsHaveAllExternalReferences(runs []BlastRunView) bool {
	hasRows := false
	for _, run := range runs {
		if len(run.Rows) == 0 {
			continue
		}
		hasRows = true
		if !blastRowsHaveAllExternalReferences(run.Rows) {
			return false
		}
	}
	return hasRows
}

func blastRunRows(runs []BlastRunView) [][]model.BlastResultRow {
	out := make([][]model.BlastResultRow, len(runs))
	for i, run := range runs {
		out[i] = run.Rows
	}
	return out
}

func parseInterProSettings(settings tui.InterProConservedRegionSettings) model.InterProConservedRegionSettings {
	defaults := model.DefaultInterProConservedRegionSettings()
	out := model.InterProConservedRegionSettings{
		UsePfamAccession:      settings.UsePfamAccession,
		UseInterProAccession:  settings.UseInterProAccession,
		UseSignatureAccession: settings.UseSignatureAccession,
		UseEntryType:          settings.UseEntryType,
		UseEntryName:          settings.UseEntryName,
		UseCoverage:           settings.UseCoverage,
		UseMatchRegions:       settings.UseMatchRegions,
	}
	out.PresentMinCoverage = parseFloatDefault(settings.PresentMinCoverage, defaults.PresentMinCoverage)
	out.PartialMinCoverage = parseFloatDefault(settings.PartialMinCoverage, defaults.PartialMinCoverage)
	out.PresentMinMatchedItems = parseIntDefault(settings.PresentMinMatchedItems, defaults.PresentMinMatchedItems)
	out.PartialMinMatchedItems = parseIntDefault(settings.PartialMinMatchedItems, defaults.PartialMinMatchedItems)
	return out
}

func parseFloatDefault(value string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func formatScientificSetting(value float64) string {
	if value <= 0 {
		return ""
	}
	if value < 0.001 || value >= 100000 {
		return strconv.FormatFloat(value, 'e', -1, 64)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func parseFloatDefaultAllowZero(value string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func parseIntDefault(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func (p *Prompter) SearchAndSelectSpecies(candidates []model.SpeciesCandidate, searchFn func(string) []model.SpeciesCandidate) (model.SpeciesCandidate, error) {
	choices := make([]tui.Choice, 0, len(candidates))
	choiceByKey := make(map[string]model.SpeciesCandidate, len(candidates))
	for i, candidate := range candidates {
		key := strconv.Itoa(i)
		choiceByKey[key] = candidate
		choices = append(choices, tui.Choice{
			Value:       key,
			Label:       candidate.DisplayLabel(),
			Description: strings.TrimSpace(candidate.JBrowseName + targetIDLabel(candidate.ProteomeID)),
		})
	}
	result, err := tui.RunSearchPage(tui.SearchPage{
		Path:        p.tuiPath("Startup", "Species search"),
		Title:       p.t("Species search"),
		Description: p.t("Search and select one species on the same page."),
		Label:       p.t("Keyword"),
		Choices:     choices,
		AllowBack:   true,
		AllowHome:   true,
		Hints:       []string{p.t("You can search by abbreviated name, full scientific name, or common name.")},
		Filter: func(query string, _ []tui.Choice) []tui.Choice {
			matches := searchFn(query)
			out := make([]tui.Choice, 0, len(matches))
			for _, candidate := range matches {
				for key, original := range choiceByKey {
					if original.JBrowseName == candidate.JBrowseName && original.GenomeLabel == candidate.GenomeLabel {
						out = append(out, tui.Choice{
							Value:       key,
							Label:       candidate.DisplayLabel(),
							Description: strings.TrimSpace(candidate.JBrowseName + targetIDLabel(candidate.ProteomeID)),
						})
						break
					}
				}
			}
			return out
		},
	})
	if err != nil {
		return model.SpeciesCandidate{}, err
	}
	if result.Nav == tui.NavBack || result.Nav == tui.NavHome {
		return model.SpeciesCandidate{}, ErrBackToDatabaseSelection
	}
	if result.Nav == tui.NavExit {
		return model.SpeciesCandidate{}, ErrExitRequested
	}
	candidate, ok := choiceByKey[result.Value]
	if !ok {
		return model.SpeciesCandidate{}, ErrExitRequested
	}
	return candidate, nil

}

func (p *Prompter) SequenceInput() (string, error) {
	result, err := tui.RunMultiLinePage(tui.MultiLinePage{
		Path:        p.blastTUIPath("BLAST input"),
		Title:       p.t("BLAST input"),
		Description: p.t("Paste one or more BLAST queries, one per line, or paste a FASTA entry / Phytozome gene or transcript report URL.") + "\n" + p.t("You can also type load \"file.txt\" to read from the program directory."),
		AllowEmpty:  true,
		AllowBack:   true,
		AllowHome:   true,
		ConfirmText: tui.ButtonRunBLAST,
		Hints:       []string{p.t("Finish sequence input with an empty line.")},
	})
	if err != nil {
		return "", err
	}
	if navErr := tuiNavError(result.Nav, ErrBackToSpeciesSelection); navErr != nil {
		return "", navErr
	}
	return result.Text, nil

}

func targetIDLabel(targetID int) string {
	if targetID == 0 {
		return ""
	}
	return fmt.Sprintf(" (target id %d)", targetID)
}

func (p *Prompter) KeywordInput() (string, error) {
	result, err := tui.RunMultiLinePage(tui.MultiLinePage{
		Path:        p.tuiPath("Startup", "Species", "Keyword input"),
		Title:       p.t("Keyword input"),
		Description: p.t("Paste one or more keywords for the selected species.") + "\n" + p.t("Separate them by spaces or new lines, then finish with an empty line."),
		AllowEmpty:  true,
		AllowBack:   true,
		AllowHome:   true,
		ConfirmText: tui.ButtonSearch,
	})
	if err != nil {
		return "", err
	}
	if navErr := tuiNavError(result.Nav, ErrBackToSpeciesSelection); navErr != nil {
		return "", navErr
	}
	return result.Text, nil

}

func (p *Prompter) SelectKeywordRows(groups []model.KeywordSearchGroup) (KeywordRowSelection, error) {
	totalRows := countKeywordResultRows(groups)
	selected := make([]bool, totalRows)
	for i := range selected {
		selected[i] = true
	}
	flatRows := make([]model.KeywordResultRow, 0, totalRows)
	for _, group := range groups {
		for _, row := range group.Rows {
			flatRows = append(flatRows, row)
		}
	}
	groupLabels := make([]string, 0, len(groups))
	for _, group := range groups {
		if label := strings.TrimSpace(group.SearchTerm); label != "" {
			groupLabels = append(groupLabels, label)
		}
	}
	columns, tableRows := buildKeywordSelectionTable(flatRows)
	stateKey := tableStateKey("keyword", columns, tableRows)
	if len(p.keywordSelection) == totalRows {
		selected = append([]bool(nil), p.keywordSelection...)
	}
	for {
		result, err := tui.RunRowSelectionPage(tui.RowSelectionPage{
			Path:         p.tuiPath("Startup", "Species", "Keyword input", "Keyword results", "Row selection"),
			Title:        p.t("Keyword results:"),
			Description:  p.t("The first result under each search term is selected by default."),
			Columns:      columns,
			Rows:         tableRows,
			Selected:     selected,
			Sort:         tui.TableSort{Column: -1, Direction: tui.SortAscending},
			GroupSort:    true,
			GroupLabels:  groupLabels,
			AllowBack:    true,
			AllowHome:    true,
			ConfirmText:  tui.ButtonView,
			GenerateText: tui.ButtonExport,
			State:        p.rowStates[stateKey],
		})
		if err != nil {
			return KeywordRowSelection{}, err
		}
		p.rowStates[stateKey] = result.State
		if navErr := tuiNavError(result.Nav, ErrBackToQueryInput); navErr != nil {
			p.keywordSelection = append([]bool(nil), result.Selected...)
			return KeywordRowSelection{}, navErr
		}
		selected = result.Selected
		p.keywordSelection = append([]bool(nil), selected...)
		chosen := make([]model.KeywordResultRow, 0, totalRows)
		for i, ok := range selected {
			if ok {
				chosen = append(chosen, flatRows[i])
			}
		}
		if len(chosen) == 0 {
			return KeywordRowSelection{}, fmt.Errorf("no rows selected")
		}
		if result.GenerateFile {
			return KeywordRowSelection{Rows: chosen, GenerateFile: true}, nil
		}
	}

}

func (p *Prompter) SelectBlastRows(rows []model.BlastResultRow) ([]model.BlastResultRow, error) {
	selection, err := p.selectBlastRows(rows, false, blastRowsBackTarget())
	return selection.Rows, err
}

func (p *Prompter) SelectBlastRowsBatch(rows []model.BlastResultRow) (BlastRowSelection, error) {
	return p.selectBlastRows(rows, true, blastRowsBackTarget())
}

func (p *Prompter) SelectBlastRowsBatchWithBack(rows []model.BlastResultRow, backTarget error) (BlastRowSelection, error) {
	return p.selectBlastRows(rows, true, backTarget)
}

func (p *Prompter) SelectBlastRowsWithOptions(rows []model.BlastResultRow, backTarget error, allowDoneAll bool) (BlastRowSelection, error) {
	return p.selectBlastRows(rows, allowDoneAll, backTarget)
}

func (p *Prompter) SelectBlastRuns(runs []BlastRunView, backTarget error) (BlastRowSelection, error) {
	items := make([]tui.BlastRunItem, 0, len(runs))
	tableKeyParts := make([]string, 0, len(runs))
	for i, run := range runs {
		columns, tableRows := buildBlastSelectionTable(run.Rows)
		selected := make([]bool, len(run.Rows))
		for r := range selected {
			selected[r] = true
		}
		tableKeyParts = append(tableKeyParts, tableStateKey(fmt.Sprintf("blast-run-%d", i), columns, tableRows))
		items = append(items, tui.BlastRunItem{
			Label:       blastRunGeneLabel(i, run.Item),
			AltLabel:    blastRunLabelName(i, run.Item),
			Description: fmt.Sprintf("%d/%d lines", len(run.Rows), len(run.Rows)),
			Columns:     columns,
			Rows:        tableRows,
			Selected:    selected,
		})
	}
	stateKey := "blast-runs:" + digestStrings(tableKeyParts)
	if cached, ok := p.blastRunSelected[stateKey]; ok && len(cached) == len(items) {
		for i := range items {
			if len(cached[i]) == len(items[i].Rows) {
				items[i].Selected = append([]bool(nil), cached[i]...)
			}
		}
	}
	if cachedFlags, ok := p.blastRunFilterFlags[stateKey]; ok && len(cachedFlags) == len(items) {
		for i := range items {
			if len(cachedFlags[i]) == len(items[i].Rows) {
				items[i].FilterFlags = append([]bool(nil), cachedFlags[i]...)
			}
		}
	}
	filterSettings := p.blastFilterSettings
	filterApplied := anyPromptFilterFlagsByRun(items)
	filterCleared := false
	for {
		result, err := tui.RunBlastRunSelectionPage(tui.BlastRunSelectionPage{
			Path:         p.blastTUIPath("BLAST input", "BLAST results", "Row selection"),
			Title:        "BLAST row selection",
			Description:  "Choose a BLAST query on the left and review its result rows on the right.",
			Items:        items,
			AllowFilter:  blastRunsHaveAllExternalReferences(runs),
			FilterText:   tui.ButtonFilter,
			AllowBack:    true,
			AllowHome:    true,
			ConfirmText:  tui.ButtonView,
			GenerateText: tui.ButtonExport,
			DoneAllText:  tui.ButtonExportAll,
			State:        p.blastRunStates[stateKey],
		})
		if err != nil {
			return BlastRowSelection{}, err
		}
		p.blastRunStates[stateKey] = result.State
		p.blastRunSelected[stateKey] = cloneBoolMatrixPrompt(result.SelectedByRun)
		p.blastRunFilterFlags[stateKey] = cloneBoolMatrixPrompt(result.FilterFlagsByRun)
		if navErr := tuiNavError(result.Nav, backTarget); navErr != nil {
			return BlastRowSelection{}, navErr
		}
		if result.FilterRequested {
			filterResult, settingsErr := p.BlastFilterSettings(ErrBackToRowSelection)
			if settingsErr != nil {
				if errors.Is(settingsErr, ErrBackToRowSelection) {
					continue
				}
				return BlastRowSelection{}, settingsErr
			}
			if filterResult.ClearFilter {
				filterSettings = filterResult.Settings
				filterApplied = false
				filterCleared = true
				rowCounts := make([]int, len(items))
				for i := range items {
					rowCounts[i] = len(items[i].Rows)
				}
				cleared, clearErr := p.clearBlastRunFiltersWithProgress(rowCounts)
				if clearErr != nil {
					if blastFilterTaskCancelled(clearErr) {
						continue
					}
					return BlastRowSelection{}, clearErr
				}
				for i := range items {
					if i < len(cleared.SelectedByRun) && len(cleared.SelectedByRun[i]) == len(items[i].Rows) {
						items[i].Selected = append([]bool(nil), cleared.SelectedByRun[i]...)
					}
					if i < len(cleared.FlagsByRun) && len(cleared.FlagsByRun[i]) == len(items[i].Rows) {
						items[i].FilterFlags = append([]bool(nil), cleared.FlagsByRun[i]...)
					}
				}
				p.blastRunSelected[stateKey] = cloneBoolMatrixPrompt(selectedByRunFromItems(items))
				p.blastRunFilterFlags[stateKey] = cloneBoolMatrixPrompt(filterFlagsByRunFromItems(items))
				result.SelectedByRun = selectedByRunFromItems(items)
				result.FilterFlagsByRun = filterFlagsByRunFromItems(items)
				result.Selected = append([]bool(nil), items[result.RunIndex].Selected...)
				continue
			}
			suggestion, suggestErr := p.blastFilterSuggestionWithProgress(BlastFilterRequest{
				RowsByRun:     blastRunRows(runs),
				SelectedByRun: result.SelectedByRun,
				Settings:      filterResult.Settings,
				CurrentRun:    result.RunIndex,
			})
			if suggestErr != nil {
				if blastFilterTaskCancelled(suggestErr) {
					continue
				}
				return BlastRowSelection{}, suggestErr
			}
			for i := range items {
				if i < len(suggestion.SelectedByRun) && len(suggestion.SelectedByRun[i]) == len(items[i].Rows) {
					items[i].Selected = append([]bool(nil), suggestion.SelectedByRun[i]...)
				}
				if i < len(suggestion.FlagsByRun) && len(suggestion.FlagsByRun[i]) == len(items[i].Rows) {
					items[i].FilterFlags = append([]bool(nil), suggestion.FlagsByRun[i]...)
				}
			}
			filterSettings = suggestion.Settings
			filterApplied = true
			filterCleared = false
			p.blastRunSelected[stateKey] = cloneBoolMatrixPrompt(suggestion.SelectedByRun)
			p.blastRunFilterFlags[stateKey] = cloneBoolMatrixPrompt(suggestion.FlagsByRun)
			continue
		}
		if result.RunIndex < 0 || result.RunIndex >= len(runs) {
			result.RunIndex = 0
		}
		rows := runs[result.RunIndex].Rows
		chosen := make([]model.BlastResultRow, 0, len(rows))
		chosenNumbers := make([]int, 0, len(rows))
		for i, ok := range result.Selected {
			if ok && i < len(rows) {
				chosen = append(chosen, rows[i])
				chosenNumbers = append(chosenNumbers, i+1)
			}
		}
		rowsByRun := make([][]model.BlastResultRow, len(runs))
		rowNumbersByRun := make([][]int, len(runs))
		for runIndex, selected := range result.SelectedByRun {
			if runIndex < 0 || runIndex >= len(runs) {
				continue
			}
			runRows := runs[runIndex].Rows
			for i, ok := range selected {
				if ok && i < len(runRows) {
					rowsByRun[runIndex] = append(rowsByRun[runIndex], runRows[i])
					rowNumbersByRun[runIndex] = append(rowNumbersByRun[runIndex], i+1)
				}
			}
		}
		filterFlags := []bool(nil)
		if result.RunIndex >= 0 && result.RunIndex < len(result.FilterFlagsByRun) {
			filterFlags = append([]bool(nil), result.FilterFlagsByRun[result.RunIndex]...)
		}
		return BlastRowSelection{
			Rows:             chosen,
			GenerateFile:     result.GenerateFile,
			DoneAll:          result.DoneAll,
			RunIndex:         result.RunIndex,
			Selected:         append([]bool(nil), result.Selected...),
			SelectedByRun:    cloneBoolMatrixPrompt(result.SelectedByRun),
			RowNumbers:       chosenNumbers,
			RowsByRun:        rowsByRun,
			RowNumbersByRun:  rowNumbersByRun,
			FilterFlags:      filterFlags,
			FilterFlagsByRun: cloneBoolMatrixPrompt(result.FilterFlagsByRun),
			FilterSettings:   filterSettings,
			FilterApplied:    filterApplied,
			FilterCleared:    filterCleared,
		}, nil
	}
}

func blastRunGeneLabel(index int, item BlastQueryItemView) string {
	for _, value := range []string{item.ProteinID, item.TranscriptID, item.GeneID, item.RawInput, item.LabelName} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return fmt.Sprintf("query %d", index+1)
}

func blastRunLabelName(index int, item BlastQueryItemView) string {
	lines := make([]string, 0)
	if label := strings.TrimSpace(item.FamilyName); label != "" {
		lines = append(lines, "["+label+"]")
	}
	if label := strings.TrimSpace(item.MemberLabel); label != "" {
		lines = append(lines, splitPromptDisplayLines(label)...)
	} else if label := strings.TrimSpace(item.LabelName); label != "" {
		lines = append(lines, label)
	}
	if len(lines) == 0 {
		return blastRunGeneLabel(index, item)
	}
	return strings.Join(uniquePromptDisplayLines(lines), "\n")
}

func splitPromptDisplayLines(value string) []string {
	raw := strings.Split(strings.ReplaceAll(strings.TrimSpace(value), "\r", ""), "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func uniquePromptDisplayLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	seen := map[string]struct{}{}
	for _, line := range lines {
		key := strings.ToLower(strings.TrimSpace(line))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, strings.TrimSpace(line))
	}
	return out
}

func blastRowsBackTarget() error {
	return ErrBackToQueryInput
}

func (p *Prompter) selectBlastRows(rows []model.BlastResultRow, allowDoneAll bool, backTarget error) (BlastRowSelection, error) {
	if len(rows) == 0 {
		return BlastRowSelection{}, nil
	}

	selected := make([]bool, len(rows))
	for i := range selected {
		selected[i] = true
	}
	columns, tableRows := buildBlastSelectionTable(rows)
	stateKey := tableStateKey("blast", columns, tableRows)
	if cached, ok := p.blastSelections[stateKey]; ok && len(cached) == len(rows) {
		selected = append([]bool(nil), cached...)
	}
	filterFlags := make([]bool, len(rows))
	if cachedFlags, ok := p.blastFilterFlags[stateKey]; ok && len(cachedFlags) == len(rows) {
		filterFlags = append([]bool(nil), cachedFlags...)
	}
	filterSettings := p.blastFilterSettings
	filterApplied := anyPromptBool(filterFlags)
	filterCleared := false
	for {
		result, err := tui.RunRowSelectionPage(tui.RowSelectionPage{
			Path:         p.blastTUIPath("BLAST input", "BLAST results", "Row selection"),
			Title:        "BLAST row selection",
			Description:  fmt.Sprintf("%d/%d rows currently selected. Review and toggle rows before choosing an action.", countSelected(selected), len(rows)),
			Columns:      columns,
			Rows:         tableRows,
			Selected:     selected,
			FilterFlags:  filterFlags,
			Sort:         tui.TableSort{Column: -1, Direction: tui.SortAscending},
			AllowFilter:  blastRowsHaveAllExternalReferences(rows),
			FilterText:   tui.ButtonFilter,
			AllowDoneAll: allowDoneAll,
			AllowBack:    true,
			AllowHome:    true,
			ConfirmText:  tui.ButtonView,
			GenerateText: tui.ButtonExport,
			DoneAllText:  tui.ButtonExportAll,
			Hints:        []string{"Ctrl+G exports selected rows"},
			State:        p.rowStates[stateKey],
		})
		if err != nil {
			return BlastRowSelection{}, err
		}
		p.rowStates[stateKey] = result.State
		if navErr := tuiNavError(result.Nav, backTarget); navErr != nil {
			p.blastSelections[stateKey] = append([]bool(nil), result.Selected...)
			p.blastFilterFlags[stateKey] = append([]bool(nil), result.FilterFlags...)
			return BlastRowSelection{}, navErr
		}
		selected = result.Selected
		filterFlags = result.FilterFlags
		p.blastSelections[stateKey] = append([]bool(nil), selected...)
		p.blastFilterFlags[stateKey] = append([]bool(nil), filterFlags...)
		if result.FilterRequested {
			filterResult, settingsErr := p.BlastFilterSettings(ErrBackToRowSelection)
			if settingsErr != nil {
				if errors.Is(settingsErr, ErrBackToRowSelection) {
					continue
				}
				return BlastRowSelection{}, settingsErr
			}
			if filterResult.ClearFilter {
				filterSettings = filterResult.Settings
				filterApplied = false
				filterCleared = true
				cleared, clearErr := p.clearBlastFilterWithProgress(len(rows))
				if clearErr != nil {
					if blastFilterTaskCancelled(clearErr) {
						continue
					}
					return BlastRowSelection{}, clearErr
				}
				if len(cleared.Selected) == len(rows) {
					selected = append([]bool(nil), cleared.Selected...)
				}
				if len(cleared.Flags) == len(rows) {
					filterFlags = append([]bool(nil), cleared.Flags...)
				}
				p.blastSelections[stateKey] = append([]bool(nil), selected...)
				p.blastFilterFlags[stateKey] = append([]bool(nil), filterFlags...)
				result.Selected = append([]bool(nil), selected...)
				result.FilterFlags = append([]bool(nil), filterFlags...)
				continue
			}
			suggestion, suggestErr := p.blastFilterSuggestionWithProgress(BlastFilterRequest{
				Rows:     rows,
				Selected: selected,
				Settings: filterResult.Settings,
			})
			if suggestErr != nil {
				if blastFilterTaskCancelled(suggestErr) {
					continue
				}
				return BlastRowSelection{}, suggestErr
			}
			if len(suggestion.Selected) == len(rows) {
				selected = append([]bool(nil), suggestion.Selected...)
			}
			if len(suggestion.Flags) == len(rows) {
				filterFlags = append([]bool(nil), suggestion.Flags...)
			}
			filterSettings = suggestion.Settings
			filterApplied = true
			filterCleared = false
			p.blastSelections[stateKey] = append([]bool(nil), selected...)
			p.blastFilterFlags[stateKey] = append([]bool(nil), filterFlags...)
			continue
		}
		chosen := make([]model.BlastResultRow, 0, len(rows))
		chosenNumbers := make([]int, 0, len(rows))
		for i, ok := range selected {
			if ok {
				chosen = append(chosen, rows[i])
				chosenNumbers = append(chosenNumbers, i+1)
			}
		}
		if len(chosen) == 0 {
			return BlastRowSelection{}, fmt.Errorf("no rows selected")
		}
		if result.GenerateFile {
			return BlastRowSelection{
				Rows:           chosen,
				GenerateFile:   true,
				DoneAll:        result.DoneAll,
				RowNumbers:     chosenNumbers,
				Selected:       append([]bool(nil), selected...),
				FilterFlags:    append([]bool(nil), result.FilterFlags...),
				FilterSettings: filterSettings,
				FilterApplied:  filterApplied,
				FilterCleared:  filterCleared,
			}, nil
		}
	}
}

func anyPromptFilterFlagsByRun(items []tui.BlastRunItem) bool {
	for _, item := range items {
		if anyPromptBool(item.FilterFlags) {
			return true
		}
	}
	return false
}

func anyPromptBool(values []bool) bool {
	for _, value := range values {
		if value {
			return true
		}
	}
	return false
}

func buildKeywordSelectionTable(rows []model.KeywordResultRow) ([]tui.TableColumn, []tui.TableRow) {
	defs := keywordDisplayColumns(rows)
	columns := make([]tui.TableColumn, 0, len(defs))
	for _, def := range defs {
		columns = append(columns, tui.TableColumn{
			ID:        def.ID,
			Header:    def.Header,
			Sortable:  def.Sortable,
			Reference: def.Reference,
			Help:      def.Help,
		})
	}
	tableRows := make([]tui.TableRow, 0, len(rows))
	for _, row := range rows {
		cells := make([]string, 0, len(defs))
		for _, def := range defs {
			cells = append(cells, def.Value(row))
		}
		tableRows = append(tableRows, tui.TableRow{Cells: cells, Group: strings.TrimSpace(row.SearchTerm), Detail: keywordRowDetail(row)})
	}
	return columns, tableRows
}

func buildBlastSelectionTable(rows []model.BlastResultRow) ([]tui.TableColumn, []tui.TableRow) {
	defs := blastDisplayColumns(rows)
	columns := make([]tui.TableColumn, 0, len(defs))
	for _, def := range defs {
		columns = append(columns, tui.TableColumn{
			ID:        def.ID,
			Header:    def.Header,
			Sortable:  def.Sortable,
			Reference: def.Reference,
			Help:      def.Help,
		})
	}
	tableRows := make([]tui.TableRow, 0, len(rows))
	for _, row := range rows {
		cells := make([]string, 0, len(defs))
		for _, def := range defs {
			cells = append(cells, def.Value(row))
		}
		tableRows = append(tableRows, tui.TableRow{Cells: cells, Detail: blastRowDetail(row)})
	}
	return columns, tableRows
}

func tableStateKey(prefix string, columns []tui.TableColumn, rows []tui.TableRow) string {
	parts := make([]string, 0, 1+len(columns)+len(rows))
	parts = append(parts, prefix)
	for _, column := range columns {
		parts = append(parts, column.ID+"\x00"+column.Header)
	}
	for _, row := range rows {
		parts = append(parts, row.Group+"\x00"+strings.Join(row.Cells, "\x00"))
	}
	return prefix + ":" + digestStrings(parts)
}

func digestStrings(parts []string) string {
	hash := sha1.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0xff})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func cloneBoolMatrixPrompt(values [][]bool) [][]bool {
	out := make([][]bool, len(values))
	for i := range values {
		out[i] = append([]bool(nil), values[i]...)
	}
	return out
}

func setAllPrompt(values []bool, value bool) {
	for i := range values {
		values[i] = value
	}
}

func selectedByRunFromItems(items []tui.BlastRunItem) [][]bool {
	out := make([][]bool, len(items))
	for i := range items {
		out[i] = append([]bool(nil), items[i].Selected...)
	}
	return out
}

func filterFlagsByRunFromItems(items []tui.BlastRunItem) [][]bool {
	out := make([][]bool, len(items))
	for i := range items {
		out[i] = append([]bool(nil), items[i].FilterFlags...)
	}
	return out
}

func blastDisplayColumns(rows []model.BlastResultRow) []tableColumnValue[model.BlastResultRow] {
	options := ColumnDisplayOptions{DatabaseDisplay: databaseDisplayNameForRows(rows), Multiline: true}
	defByID := map[string]tableColumnValue[model.BlastResultRow]{
		"label_name": {
			ID:       "label_name",
			Header:   ColumnCompactHeader("label_name", options),
			Sortable: true,
			Value: func(row model.BlastResultRow) string {
				return blastLabelName(row)
			},
		},
		"protein": {
			ID:       "protein",
			Header:   ColumnCompactHeader("protein", options),
			Sortable: true,
			Value: func(row model.BlastResultRow) string {
				return firstNonEmptyText(row.Protein, row.SubjectID)
			},
		},
		"percent_identity": {
			ID:       "percent_identity",
			Header:   ColumnCompactHeader("percent_identity", options),
			Sortable: true,
			Value: func(row model.BlastResultRow) string {
				return fmt.Sprintf("%.2f", row.PercentIdentity)
			},
		},
		"e_value": {
			ID:       "e_value",
			Header:   ColumnCompactHeader("e_value", options),
			Sortable: true,
			Value: func(row model.BlastResultRow) string {
				return strings.TrimSpace(row.EValue)
			},
		},
		"uniprot_accession": {
			ID:        "uniprot_accession",
			Header:    ColumnCompactHeader("uniprot_accession", options),
			Sortable:  true,
			Reference: "uniprot",
			Value: func(row model.BlastResultRow) string {
				if !blastRowHasUniProtData(row) {
					return ""
				}
				return strings.TrimSpace(row.UniProtAccession)
			},
		},
		"uniprot_reviewed": {
			ID:        "uniprot_reviewed",
			Header:    ColumnCompactHeader("uniprot_reviewed", options),
			Sortable:  true,
			Reference: "uniprot",
			Value: func(row model.BlastResultRow) string {
				if !blastRowHasUniProtData(row) {
					return ""
				}
				return strings.TrimSpace(row.UniProtReviewed)
			},
		},
		"uniprot_protein_name": {
			ID:        "uniprot_protein_name",
			Header:    ColumnCompactHeader("uniprot_protein_name", options),
			Sortable:  true,
			Reference: "uniprot",
			Value: func(row model.BlastResultRow) string {
				if !blastRowHasUniProtData(row) {
					return ""
				}
				return strings.TrimSpace(row.UniProtProteinName)
			},
		},
		"uniprot_gene_names": {
			ID:        "uniprot_gene_names",
			Header:    ColumnCompactHeader("uniprot_gene_names", options),
			Sortable:  true,
			Reference: "uniprot",
			Value: func(row model.BlastResultRow) string {
				if !blastRowHasUniProtData(row) {
					return ""
				}
				return strings.TrimSpace(row.UniProtGeneNames)
			},
		},
		"uniprot_keywords": {
			ID:        "uniprot_keywords",
			Header:    ColumnCompactHeader("uniprot_keywords", options),
			Sortable:  true,
			Reference: "uniprot",
			Value: func(row model.BlastResultRow) string {
				if !blastRowHasUniProtData(row) {
					return ""
				}
				return strings.TrimSpace(row.UniProtKeywords)
			},
		},
		"uniprot_ec": {
			ID:        "uniprot_ec",
			Header:    ColumnCompactHeader("uniprot_ec", options),
			Sortable:  true,
			Reference: "uniprot",
			Value: func(row model.BlastResultRow) string {
				if !blastRowHasUniProtData(row) {
					return ""
				}
				return strings.TrimSpace(row.UniProtEC)
			},
		},
		"uniprot_go": {
			ID:        "uniprot_go",
			Header:    ColumnCompactHeader("uniprot_go", options),
			Sortable:  true,
			Reference: "uniprot",
			Value: func(row model.BlastResultRow) string {
				if !blastRowHasUniProtData(row) {
					return ""
				}
				return strings.TrimSpace(row.UniProtGO)
			},
		},
		"target_uniprot_canonical_length_percent": {
			ID:        "target_uniprot_canonical_length_percent",
			Header:    ColumnCompactHeader("target_uniprot_canonical_length_percent", options),
			Sortable:  true,
			Reference: "uniprot",
			Value: func(row model.BlastResultRow) string {
				if !blastRowHasUniProtData(row) {
					return ""
				}
				return strings.TrimSpace(row.TargetUniProtCanonicalLengthPercent)
			},
		},
		"align_query_length_percent": {
			ID:       "align_query_length_percent",
			Header:   ColumnCompactHeader("align_query_length_percent", options),
			Sortable: true,
			Value: func(row model.BlastResultRow) string {
				return blastAlignQueryLengthPercent(row)
			},
		},
		"interpro_conserved_region_status": {
			ID:        "interpro_conserved_region_status",
			Header:    ColumnCompactHeader("interpro_conserved_region_status", options),
			Sortable:  true,
			Reference: "interpro",
			Value: func(row model.BlastResultRow) string {
				if !row.InterProReferenceEnabled {
					return ""
				}
				return strings.TrimSpace(row.InterProConservedRegionStatus)
			},
		},
		"target_length": {
			ID:       "target_length",
			Header:   ColumnCompactHeader("target_length", options),
			Sortable: true,
			Value: func(row model.BlastResultRow) string {
				return fmt.Sprintf("%d", row.TargetLength)
			},
		},
		"align_len": {
			ID:       "align_len",
			Header:   ColumnCompactHeader("align_len", options),
			Sortable: true,
			Value: func(row model.BlastResultRow) string {
				return fmt.Sprintf("%d", row.AlignLength)
			},
		},
		"query_length": {
			ID:       "query_length",
			Header:   ColumnCompactHeader("query_length", options),
			Sortable: true,
			Value: func(row model.BlastResultRow) string {
				return fmt.Sprintf("%d", row.QueryLength)
			},
		},
		"uniprot_canonical_length": {
			ID:        "uniprot_canonical_length",
			Header:    ColumnCompactHeader("uniprot_canonical_length", options),
			Sortable:  true,
			Reference: "uniprot",
			Value: func(row model.BlastResultRow) string {
				if !blastRowHasUniProtData(row) {
					return ""
				}
				return strings.TrimSpace(row.UniProtCanonicalLength)
			},
		},
		"interpro_entry_name": {
			ID:        "interpro_entry_name",
			Header:    ColumnCompactHeader("interpro_entry_name", options),
			Sortable:  true,
			Reference: "interpro",
			Value: func(row model.BlastResultRow) string {
				if !blastRowHasInterProData(row) {
					return ""
				}
				return strings.TrimSpace(row.InterProEntryName)
			},
		},
		"interpro_entry_type": {
			ID:        "interpro_entry_type",
			Header:    ColumnCompactHeader("interpro_entry_type", options),
			Sortable:  true,
			Reference: "interpro",
			Value: func(row model.BlastResultRow) string {
				if !blastRowHasInterProData(row) {
					return ""
				}
				return strings.TrimSpace(row.InterProEntryType)
			},
		},
		"interpro_coverage_percent": {
			ID:        "interpro_coverage_percent",
			Header:    ColumnCompactHeader("interpro_coverage_percent", options),
			Sortable:  true,
			Reference: "interpro",
			Value: func(row model.BlastResultRow) string {
				if !blastRowHasInterProData(row) {
					return ""
				}
				return strings.TrimSpace(row.InterProCoveragePercent)
			},
		},
		"interpro_match_regions": {
			ID:        "interpro_match_regions",
			Header:    ColumnCompactHeader("interpro_match_regions", options),
			Sortable:  true,
			Reference: "interpro",
			Value: func(row model.BlastResultRow) string {
				if !blastRowHasInterProData(row) {
					return ""
				}
				return strings.TrimSpace(row.InterProMatchRegions)
			},
		},
		"species": {
			ID:       "species",
			Header:   ColumnCompactHeader("species", options),
			Sortable: true,
			Value: func(row model.BlastResultRow) string {
				return row.Species
			},
		},
	}
	ids := blastDisplayColumnIDsForRows(rows)
	defs := make([]tableColumnValue[model.BlastResultRow], 0, len(ids))
	for _, id := range ids {
		if id == "label_name" && !blastRowsHaveLabelName(rows) {
			continue
		}
		if def, ok := defByID[id]; ok {
			if def.Help == "" {
				def.Help = ColumnHelpText(id)
			}
			defs = append(defs, def)
		}
	}
	return defs
}

func blastDisplayColumnIDsForRows(rows []model.BlastResultRow) []string {
	withUniProt := blastRowsHaveUniProtReference(rows)
	withInterPro := blastRowsHaveInterProReference(rows)
	return BlastDisplayColumnIDs(sourceDatabaseForBlastRows(rows), blastRowsProgram(rows), withUniProt, withInterPro)
}

func databaseDisplayNameForRows(rows []model.BlastResultRow) string {
	for _, row := range rows {
		switch strings.ToLower(strings.TrimSpace(row.SourceDatabase)) {
		case "lemna":
			return "lemna"
		case "phytozome":
			return "Phytozome"
		case "":
			continue
		default:
			return strings.TrimSpace(row.SourceDatabase)
		}
	}
	return "target"
}

func blastDetailColumnIDsForRow(row model.BlastResultRow) []string {
	return BlastDetailColumnIDs(strings.TrimSpace(row.SourceDatabase), strings.TrimSpace(row.BlastProgram), row.UniProtReferenceEnabled, row.InterProReferenceEnabled)
}

func blastRowsHaveUniProtReference(rows []model.BlastResultRow) bool {
	for _, row := range rows {
		if row.UniProtReferenceEnabled {
			return true
		}
	}
	return false
}

func blastRowHasUniProtData(row model.BlastResultRow) bool {
	return strings.TrimSpace(row.UniProtAccession) != ""
}

func blastColumnIsUniProtReference(id string) bool {
	return blastColumnIsUniProtLike(id)
}

func blastRowsHaveInterProReference(rows []model.BlastResultRow) bool {
	for _, row := range rows {
		if row.InterProReferenceEnabled {
			return true
		}
	}
	return false
}

func blastRowHasInterProData(row model.BlastResultRow) bool {
	return strings.TrimSpace(row.InterProAccessions) != "" ||
		strings.TrimSpace(row.InterProEntryName) != "" ||
		strings.TrimSpace(row.InterProConservedRegionStatus) != ""
}

func blastColumnIsInterProReference(id string) bool {
	return blastColumnIsInterProLike(id)
}

func blastRowsFromLemna(rows []model.BlastResultRow) bool {
	return strings.EqualFold(sourceDatabaseForBlastRows(rows), "lemna")
}

func blastRowsProgram(rows []model.BlastResultRow) string {
	for _, row := range rows {
		if program := strings.ToUpper(strings.TrimSpace(row.BlastProgram)); program != "" {
			return program
		}
	}
	return ""
}

func blastRowsHaveLabelName(rows []model.BlastResultRow) bool {
	for _, row := range rows {
		if blastLabelName(row) != "" {
			return true
		}
	}
	return false
}

func keywordDisplayColumns(rows []model.KeywordResultRow) []tableColumnValue[model.KeywordResultRow] {
	options := ColumnDisplayOptions{DatabaseDisplay: "keyword", Multiline: true}
	defByID := map[string]tableColumnValue[model.KeywordResultRow]{
		"search_term": {
			ID:       "search_term",
			Header:   ColumnCompactHeader("search_term", options),
			Sortable: false,
			Value: func(row model.KeywordResultRow) string {
				return row.SearchTerm
			},
		},
		"label_name": {
			ID:       "label_name",
			Header:   ColumnCompactHeader("label_name", options),
			Sortable: true,
			Value: func(row model.KeywordResultRow) string {
				return keywordLabelName(row)
			},
		},
		"transcript": {
			ID:       "transcript",
			Header:   ColumnCompactHeader("transcript", options),
			Sortable: true,
			Value: func(row model.KeywordResultRow) string {
				return row.TranscriptID
			},
		},
		"description": {
			ID:       "description",
			Header:   ColumnCompactHeader("description", options),
			Sortable: true,
			Value: func(row model.KeywordResultRow) string {
				return row.Description
			},
		},
		"genome": {
			ID:       "genome",
			Header:   ColumnCompactHeader("genome", options),
			Sortable: true,
			Value: func(row model.KeywordResultRow) string {
				return row.Genome
			},
		},
	}
	ids := keywordDisplayColumnIDsForRows(rows)
	defs := make([]tableColumnValue[model.KeywordResultRow], 0, len(ids))
	for _, id := range ids {
		if id == "label_name" && !keywordRowsHaveLabelName(rows) {
			continue
		}
		if def, ok := defByID[id]; ok {
			if def.Help == "" {
				def.Help = ColumnHelpText(id)
			}
			defs = append(defs, def)
		}
	}
	return defs
}

func phytozomeKeywordDisplayColumns(rows []model.KeywordResultRow) []tableColumnValue[model.KeywordResultRow] {
	return keywordDisplayColumns(rows)
}

func keywordDisplayColumnIDsForRows(rows []model.KeywordResultRow) []string {
	return KeywordDisplayColumnIDs(sourceDatabaseForKeywordRows(rows))
}

func keywordDetailColumnIDsForRow(row model.KeywordResultRow) []string {
	return KeywordDetailColumnIDs(strings.TrimSpace(row.SourceDatabase))
}

func keywordRowsFromLemna(rows []model.KeywordResultRow) bool {
	return strings.EqualFold(sourceDatabaseForKeywordRows(rows), "lemna")
}

func keywordRowsHaveLabelName(rows []model.KeywordResultRow) bool {
	for _, row := range rows {
		if keywordLabelName(row) != "" {
			return true
		}
	}
	return false
}

func keywordRowsHaveProteinID(rows []model.KeywordResultRow) bool {
	for _, row := range rows {
		if strings.TrimSpace(row.ProteinID) != "" {
			return true
		}
	}
	return false
}

func sourceDatabaseForBlastRows(rows []model.BlastResultRow) string {
	for _, row := range rows {
		if value := strings.TrimSpace(row.SourceDatabase); value != "" {
			return value
		}
	}
	return "phytozome"
}

func sourceDatabaseForKeywordRows(rows []model.KeywordResultRow) string {
	for _, row := range rows {
		if value := strings.TrimSpace(row.SourceDatabase); value != "" {
			return value
		}
	}
	return "phytozome"
}

func (p *Prompter) ExportBaseName(label string, backTarget error) (string, error) {
	promptLabel := strings.TrimSpace(label)
	if promptLabel == "" {
		promptLabel = "Export file name"
	}
	result, err := tui.RunTextInputPage(tui.TextInputPage{
		Path:        p.tuiPath("Startup", "Export", "File name"),
		Title:       promptLabel,
		Description: p.t("Enter one base name without extension. The program will create both '<name>.xlsx' and '<name>.txt'."),
		Label:       p.t("File name"),
		AllowEmpty:  false,
		AllowBack:   true,
		AllowHome:   true,
		ConfirmText: tui.ButtonSave,
	})
	if err != nil {
		return "", err
	}
	if navErr := tuiNavError(result.Nav, backTarget); navErr != nil {
		return "", navErr
	}
	name := sanitizeFileName(result.Text)
	if name == "" {
		return "", fmt.Errorf("file name cannot be empty")
	}
	return name, nil

}

func (p *Prompter) ExportSettings(label string, allowFolder bool, allowEmptyFileName bool, mentionBlastHeaderFallback bool, backTarget error) (ExportSettings, error) {
	promptLabel := strings.TrimSpace(label)
	if promptLabel == "" {
		promptLabel = "Export file name"
	}
	message := "Enter export settings before generating files."
	if mentionBlastHeaderFallback {
		message = "Enter export settings before generating files. If this BLAST query has no label name, this file name will also be used inside the TXT/FASTA title header; using a gene label name is recommended."
	}
	result, err := tui.RunExportSettingsModal(tui.ExportSettingsPage{
		Path:           p.tuiPath("Startup", "Export", "Settings"),
		Title:          p.t("Export settings"),
		Message:        p.t(message),
		FileLabel:      p.t(promptLabel),
		FolderLabel:    p.t("Output folder"),
		AllowFolder:    allowFolder,
		AllowEmptyFile: allowEmptyFileName,
		ReportLabel:    p.t("Data analysis report (PDF)"),
		AllowBack:      true,
		AllowHome:      true,
		ConfirmText:    tui.ButtonExport,
		WriteText:      true,
		WriteExcel:     true,
		WriteRawExcel:  false,
	})
	if err != nil {
		return ExportSettings{}, err
	}
	if navErr := tuiNavError(result.Nav, backTarget); navErr != nil {
		return ExportSettings{}, navErr
	}
	name := sanitizeFileName(result.FileName)
	if name == "" && !allowEmptyFileName {
		return ExportSettings{}, fmt.Errorf("file name cannot be empty")
	}
	return ExportSettings{
		BaseName:      name,
		FolderName:    sanitizeFileName(result.FolderName),
		WriteReport:   result.WriteReport,
		WriteText:     result.WriteText,
		WriteExcel:    result.WriteExcel,
		WriteRawExcel: result.WriteRawExcel,
	}, nil
}

// PostRunAction prompts the user for what to do after a completed run.
// It returns one of:
//   - "change_query"   : enter a new query for the same species
//   - "change_species" : go back to species search/selection within the current mode
//   - "home"           : return to database/mode selection
//   - "exit"           : quit the wizard
func (p *Prompter) PostRunAction(mode string, lemnaMode bool, backTarget error) (string, error) {
	modeLabel := strings.TrimSpace(mode)
	if modeLabel == "" {
		modeLabel = "workflow"
	}
	inputLabel := "Re-enter keyword"
	if strings.EqualFold(modeLabel, "blast") {
		inputLabel = "Re-enter BLAST"
	}
	speciesLabel := "Search species again"
	if lemnaMode {
		speciesLabel = "Select species again"
	}
	result, err := tui.RunChoiceModalPage(tui.ChoiceModalPage{
		Path:    p.tuiPath("Startup", "Run complete", "Next action"),
		Title:   p.t("What would you like to do next?"),
		Message: p.t("Choose how to continue after this completed run."),
		Choices: []tui.Choice{
			{
				Value:       "change_query",
				Label:       inputLabel,
				Description: fmt.Sprintf("Enter a new %s query for the same species", modeLabel),
			},
			{
				Value:       "change_species",
				Label:       speciesLabel,
				Description: fmt.Sprintf("Choose another species and run %s", modeLabel),
			},
			{
				Value:       "exit",
				Label:       "Exit",
				Description: "Exit",
			},
		},
		ConfirmText: tui.ButtonSelect,
		AllowClose:  true,
	})
	if err != nil {
		return "", err
	}
	if result.Nav == tui.NavBack {
		return "stay", nil
	}
	if result.Value == "close" {
		return "stay", nil
	}
	if result.Value == "" {
		return "exit", nil
	}
	return result.Value, nil

}

func toggleSelections(selected []bool, order []int, fields []string) error {
	for _, field := range fields {
		indexes, err := parseRowSpec(field, len(order))
		if err != nil {
			return err
		}
		for _, displayIndex := range indexes {
			rowIndex := order[displayIndex-1]
			selected[rowIndex] = !selected[rowIndex]
		}
	}
	return nil
}

func applySelectionCommand(selected []bool, order []int, args []string, value bool) error {
	if len(args) == 0 {
		return fmt.Errorf("missing selection arguments")
	}

	switch args[0] {
	case "up", "down":
		if len(args) != 2 {
			return fmt.Errorf("use '%s <row>' with exactly one row number", args[0])
		}
		index, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid row number %q", args[1])
		}
		if index < 1 || index > len(order) {
			return fmt.Errorf("row %d out of range", index)
		}

		var indexes []int
		if args[0] == "up" {
			indexes = make([]int, 0, index)
			for i := 1; i <= index; i++ {
				indexes = append(indexes, i)
			}
		} else {
			indexes = make([]int, 0, len(order)-index+1)
			for i := index; i <= len(order); i++ {
				indexes = append(indexes, i)
			}
		}
		setDisplayIndexes(selected, order, indexes, value)
		return nil
	default:
		indexes := make([]int, 0, len(args))
		for _, arg := range args {
			parsed, err := parseRowSpec(arg, len(order))
			if err != nil {
				return err
			}
			indexes = append(indexes, parsed...)
		}
		setDisplayIndexes(selected, order, indexes, value)
		return nil
	}
}

func setDisplayIndexes(selected []bool, order []int, indexes []int, value bool) {
	for _, displayIndex := range indexes {
		rowIndex := order[displayIndex-1]
		selected[rowIndex] = value
	}
}

func countSelected(selected []bool) int {
	total := 0
	for _, value := range selected {
		if value {
			total++
		}
	}
	return total
}

func parseRowSpec(spec string, max int) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty row spec")
	}
	if strings.Contains(spec, "~") {
		parts := strings.Split(spec, "~")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range %q", spec)
		}
		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid range start %q", parts[0])
		}
		end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid range end %q", parts[1])
		}
		if start < 1 || end < 1 || start > max || end > max {
			return nil, fmt.Errorf("range %q out of bounds", spec)
		}
		if start > end {
			start, end = end, start
		}
		indexes := make([]int, 0, end-start+1)
		for i := start; i <= end; i++ {
			indexes = append(indexes, i)
		}
		return indexes, nil
	}

	index, err := strconv.Atoi(spec)
	if err != nil {
		return nil, fmt.Errorf("invalid row number %q", spec)
	}
	if index < 1 || index > max {
		return nil, fmt.Errorf("row %d out of range", index)
	}
	return []int{index}, nil
}

func commandTargetValue(command string) (bool, bool) {
	switch command {
	case "on", "select":
		return true, true
	case "off", "unselect":
		return false, true
	default:
		return false, false
	}
}

func defaultRowOrder(size int) []int {
	order := make([]int, size)
	for i := range order {
		order[i] = i
	}
	return order
}

func identityRowOrder(rows []model.BlastResultRow) []int {
	order := defaultRowOrder(len(rows))
	sort.SliceStable(order, func(i, j int) bool {
		left := rows[order[i]]
		right := rows[order[j]]
		if left.PercentIdentity != right.PercentIdentity {
			return left.PercentIdentity > right.PercentIdentity
		}
		return order[i] < order[j]
	})
	return order
}

// FetchErrorAction prompts the user when a fetch (gene/sequence) operation fails.
func (p *Prompter) FetchErrorAction(description string, backTarget error) (string, error) {
	return p.recoveryErrorAction(
		p.tuiPath("Startup", "Recovery", "Fetch error"),
		p.t("Fetch error"),
		"Failed to fetch: "+description,
		true,
		backTarget,
	)
}

// WorkflowErrorAction prompts the user when a higher-level wizard step fails.
func (p *Prompter) WorkflowErrorAction(description string, backTarget error) (string, error) {
	return p.recoveryErrorAction(
		p.tuiPath("Startup", "Recovery", "Workflow error"),
		p.t("Workflow error"),
		"Step failed: "+description,
		false,
		backTarget,
	)
}

// BlastSubmitErrorAction prompts the user after a BLAST submission failure.
func (p *Prompter) BlastSubmitErrorAction(description string) (string, error) {
	return p.recoveryErrorAction(
		p.tuiPath("Startup", "Recovery", "BLAST submit error"),
		p.t("BLAST submit error"),
		"Step failed: "+description,
		false,
		ErrBackToQueryInput,
	)
}

func (p *Prompter) recoveryErrorAction(path []string, title string, message string, allowSkip bool, backTarget error) (string, error) {
	result, err := tui.RunRecoveryModalPage(tui.RecoveryModalPage{
		Path:      path,
		Title:     title,
		Message:   message,
		AllowSkip: allowSkip,
	})
	if err != nil {
		return "", err
	}
	if navErr := tuiNavError(result.Nav, backTarget); navErr != nil {
		return "", navErr
	}
	if result.Value == "" {
		return "close", nil
	}
	return result.Value, nil
}

// BlastPlusInstallAction prompts the user when local BLAST needs BLAST+.
// It returns:
//   - "install" : download and install managed BLAST+ for this app
//   - "back"    : return to BLAST query input
func (p *Prompter) BlastPlusInstallAction(description string) (string, error) {
	result, err := tui.RunActionModalPage(tui.ActionModalPage{
		Path:    p.tuiPath("Startup", "Recovery", "Fetch error"),
		Title:   p.t("Fetch error"),
		Message: "Failed to fetch: " + description + "\n\nNCBI BLAST+ is required. Install a managed copy now and continue the current operation?",
		Actions: []tui.Action{
			{Value: "close", Label: tui.ButtonClose},
		},
		ConfirmText:  tui.ButtonInstall,
		ConfirmValue: "install",
	})
	if err != nil {
		return "", err
	}
	if navErr := tuiNavError(result.Nav, ErrBackToQueryInput); navErr != nil {
		return "", navErr
	}
	if result.Value == "close" || result.Value == "" {
		return "", ErrDialogClosed
	}
	if result.Value != "" {
		return result.Value, nil
	}
	return "", ErrDialogClosed

}

func parseKeywordIdentityValues(lines []string) []string {
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		for _, token := range strings.Fields(line) {
			if token == "~" {
				values = append(values, "")
				continue
			}
			values = append(values, token)
		}
	}
	return values
}

func countKeywordResultRows(groups []model.KeywordSearchGroup) int {
	total := 0
	for _, group := range groups {
		total += len(group.Rows)
	}
	return total
}

func parseBlastIdentityValues(lines []string) []string {
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "~" {
			values = append(values, "")
			continue
		}
		values = append(values, strings.TrimSpace(line))
	}
	return values
}

func keywordLabelName(row model.KeywordResultRow) string {
	return strings.TrimSpace(row.LabelName)
}

func blastLabelName(row model.BlastResultRow) string {
	return strings.TrimSpace(row.LabelName)
}

func keywordRowDetail(row model.KeywordResultRow) string {
	values := map[string]string{
		"search_term":           row.SearchTerm,
		"label_name":            keywordLabelName(row),
		"protein_id":            row.ProteinID,
		"transcript":            row.TranscriptID,
		"gene_identifier":       row.GeneIdentifier,
		"genome":                row.Genome,
		"location":              row.Location,
		"alias":                 row.Aliases,
		"uniprot":               row.UniProt,
		"description":           row.Description,
		"comments":              row.Comments,
		"auto_define":           row.AutoDefine,
		"gene_report_url":       row.GeneReportURL,
		"sequence_header_label": row.SequenceHeaderLabel,
		"sequence_id":           row.SequenceID,
	}
	detailIDs := keywordDetailColumnIDsForRow(row)
	lines := make([]string, 0, len(detailIDs)+len(row.ExtraColumns)+2)
	for _, id := range detailIDs {
		if id == "protein_id" && strings.TrimSpace(values[id]) == "" {
			continue
		}
		lines = append(lines, ColumnDetailLabel(id, ColumnDisplayOptions{})+": "+displayPreviewValue(values[id]))
	}
	if len(row.ExtraColumns) > 0 {
		keys := make([]string, 0, len(row.ExtraColumns))
		for key := range row.ExtraColumns {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		lines = append(lines, "", "extra_columns:")
		for _, key := range keys {
			lines = append(lines, ColumnDetailLabel(key, ColumnDisplayOptions{})+": "+displayPreviewValue(row.ExtraColumns[key]))
		}
	}
	return strings.Join(lines, "\n")
}

func blastRowDetail(row model.BlastResultRow) string {
	values := map[string]string{
		"source_database":                         row.SourceDatabase,
		"blast_program":                           row.BlastProgram,
		"label_name":                              blastLabelName(row),
		"hit_number":                              fmt.Sprintf("%d", row.HitNumber),
		"hsp_number":                              fmt.Sprintf("%d", row.HSPNumber),
		"protein":                                 row.Protein,
		"subject_id":                              firstNonEmptyText(row.SubjectID, row.Protein),
		"species":                                 row.Species,
		"e_value":                                 row.EValue,
		"percent_identity":                        fmt.Sprintf("%.2f", row.PercentIdentity),
		"uniprot_accession":                       strings.TrimSpace(row.UniProtAccession),
		"uniprot_entry_name":                      strings.TrimSpace(row.UniProtEntryName),
		"uniprot_reviewed":                        strings.TrimSpace(row.UniProtReviewed),
		"uniprot_protein_name":                    strings.TrimSpace(row.UniProtProteinName),
		"uniprot_gene_names":                      strings.TrimSpace(row.UniProtGeneNames),
		"uniprot_organism":                        strings.TrimSpace(row.UniProtOrganism),
		"uniprot_organism_id":                     strings.TrimSpace(row.UniProtOrganismID),
		"uniprot_keywords":                        strings.TrimSpace(row.UniProtKeywords),
		"uniprot_ec":                              strings.TrimSpace(row.UniProtEC),
		"uniprot_go":                              strings.TrimSpace(row.UniProtGO),
		"uniprot_go_ids":                          strings.TrimSpace(row.UniProtGOIDs),
		"uniprot_function":                        strings.TrimSpace(row.UniProtFunction),
		"uniprot_catalytic_activity":              strings.TrimSpace(row.UniProtCatalyticActivity),
		"uniprot_pathway":                         strings.TrimSpace(row.UniProtPathway),
		"uniprot_subcellular_location":            strings.TrimSpace(row.UniProtSubcellularLocation),
		"uniprot_protein_existence":               strings.TrimSpace(row.UniProtProteinExistence),
		"uniprot_annotation_score":                strings.TrimSpace(row.UniProtAnnotationScore),
		"uniprot_fragment":                        strings.TrimSpace(row.UniProtFragment),
		"uniprot_sequence_caution":                strings.TrimSpace(row.UniProtSequenceCaution),
		"uniprot_pfam":                            strings.TrimSpace(row.UniProtPfam),
		"uniprot_interpro":                        strings.TrimSpace(row.UniProtInterPro),
		"uniprot_domain":                          strings.TrimSpace(row.UniProtDomain),
		"uniprot_region":                          strings.TrimSpace(row.UniProtRegion),
		"uniprot_motif":                           strings.TrimSpace(row.UniProtMotif),
		"uniprot_active_site":                     strings.TrimSpace(row.UniProtActiveSite),
		"uniprot_binding_site":                    strings.TrimSpace(row.UniProtBindingSite),
		"uniprot_alphafolddb":                     strings.TrimSpace(row.UniProtAlphaFoldDB),
		"uniprot_pdb":                             strings.TrimSpace(row.UniProtPDB),
		"interpro_conserved_region_status":        strings.TrimSpace(row.InterProConservedRegionStatus),
		"interpro_entry_name":                     strings.TrimSpace(row.InterProEntryName),
		"interpro_entry_type":                     strings.TrimSpace(row.InterProEntryType),
		"interpro_coverage_percent":               strings.TrimSpace(row.InterProCoveragePercent),
		"interpro_match_regions":                  strings.TrimSpace(row.InterProMatchRegions),
		"interpro_accessions":                     strings.TrimSpace(row.InterProAccessions),
		"interpro_signature_accessions":           strings.TrimSpace(row.InterProSignatureAccessions),
		"interpro_pfam_accessions":                strings.TrimSpace(row.InterProPfamAccessions),
		"target_uniprot_canonical_length_percent": strings.TrimSpace(row.TargetUniProtCanonicalLengthPercent),
		"align_query_length_percent":              blastAlignQueryLengthPercent(row),
		"target_length":                           fmt.Sprintf("%d", row.TargetLength),
		"align_len":                               fmt.Sprintf("%d", row.AlignLength),
		"strands":                                 row.Strands,
		"query_id":                                row.QueryID,
		"query_from":                              fmt.Sprintf("%d", row.QueryFrom),
		"query_to":                                fmt.Sprintf("%d", row.QueryTo),
		"target_from":                             fmt.Sprintf("%d", row.TargetFrom),
		"target_to":                               fmt.Sprintf("%d", row.TargetTo),
		"bitscore":                                fmt.Sprintf("%.2f", row.Bitscore),
		"mismatches":                              fmt.Sprintf("%d", row.Mismatches),
		"gap_openings":                            fmt.Sprintf("%d", row.GapOpenings),
		"identical":                               fmt.Sprintf("%d", row.Identical),
		"positives":                               fmt.Sprintf("%d", row.Positives),
		"gaps":                                    fmt.Sprintf("%d", row.Gaps),
		"query_length":                            fmt.Sprintf("%d", row.QueryLength),
		"uniprot_canonical_length":                strings.TrimSpace(row.UniProtCanonicalLength),
		"jbrowse_name":                            row.JBrowseName,
		"target_id":                               fmt.Sprintf("%d", row.TargetID),
		"sequence_id":                             row.SequenceID,
		"transcript_id":                           row.TranscriptID,
		"defline":                                 row.Defline,
		"gene_report_url":                         row.GeneReportURL,
	}
	ids := blastDetailColumnIDsForRow(row)
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		lines = append(lines, blastDetailLabel(id, row)+": "+blastDetailDisplayValue(id, row, values[id]))
	}
	return strings.Join(lines, "\n")
}

func blastDetailDisplayValue(id string, row model.BlastResultRow, value string) string {
	if blastColumnIsUniProtReference(id) && !blastRowHasUniProtData(row) {
		return ""
	}
	if blastColumnIsInterProReference(id) && !blastRowHasInterProData(row) {
		return ""
	}
	return displayPreviewValue(value)
}

func blastDetailLabel(id string, row model.BlastResultRow) string {
	return ColumnDetailLabel(id, ColumnDisplayOptions{DatabaseDisplay: databaseDisplayNameForRows([]model.BlastResultRow{row})})
}

func displayPreviewValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "~"
	}
	return value
}

func sanitizeFileName(value string) string {
	value = strings.TrimSpace(value)
	value = invalidFileNameChars.ReplaceAllString(value, "_")
	value = strings.Trim(value, ". ")
	return value
}

func looksLikeURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		return err == nil && parsed.Host != ""
	}
	if strings.Contains(value, "phytozome-next.jgi.doe.gov/") {
		parsed, err := url.Parse("https://" + value)
		return err == nil && parsed.Host != ""
	}
	return false
}
