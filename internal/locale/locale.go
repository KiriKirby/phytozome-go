package locale

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Language string

const (
	English  Language = "en"
	Chinese  Language = "cn"
	Japanese Language = "jp"
)

func Parse(value string) (Language, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "en", "eng", "english":
		return English, true
	case "cn", "zh", "zh-cn", "zh-hans", "chinese", "中文":
		return Chinese, true
	case "jp", "ja", "ja-jp", "japanese", "日本語":
		return Japanese, true
	default:
		return English, false
	}
}

func DetectFromExecutable(executableName string) Language {
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(executableName), filepath.Ext(executableName)))
	switch {
	case strings.HasSuffix(base, "-cn"):
		return Chinese
	case strings.HasSuffix(base, "-jp"):
		return Japanese
	case strings.HasSuffix(base, "-en"):
		return English
	default:
		return English
	}
}

func DetectFromArgs(args []string) (Language, bool, []string) {
	lang := English
	found := false
	kept := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(arg)), "lang=") {
			if parsed, ok := Parse(strings.TrimSpace(arg[5:])); ok {
				lang = parsed
				found = true
			}
			continue
		}
		kept = append(kept, arg)
	}
	return lang, found, kept
}

var translations = map[Language]map[string]string{
	Chinese: {
		"Global navigation: back - previous page | spawn - mode selection | lobby - database selection | exit - quit the wizard": "全局导航：back - 返回上一页 | spawn - 回到模式选择 | lobby - 回到数据库选择 | exit - 退出向导",
		"Database selection:":                                                           "数据库选择：",
		"Mode selection:":                                                               "模式选择：",
		"BLAST program selection:":                                                      "BLAST 程序选择：",
		"BLAST execution target:":                                                       "BLAST 执行方式：",
		"Enter a species keyword: ":                                                     "输入物种关键词：",
		"Select 1 or 2 (or 'phytozome'/'lemna'): ":                                      "输入 1 或 2（或 'phytozome'/'lemna'）：",
		"Select 1 or 2 (or 'blast'/'keyword'): ":                                        "输入 1 或 2（或 'blast'/'keyword'）：",
		"Select a program by number or name (type help for details): ":                  "输入程序编号或名称（输入 help 查看说明）：",
		"Select 1 or 2 (or 'server'/'local'): ":                                         "输入 1 或 2（或 'server'/'local'）：",
		"Enter one label per search term.":                                              "每个搜索词输入一个标签。",
		"Use ~ for a blank label.":                                                      "用 ~ 表示空标签。",
		"Press Enter on the first line to skip all labels.":                             "第一行直接回车可跳过所有标签。",
		"Enter one label for this BLAST query, or press Enter to skip.":                 "为这条 BLAST 查询输入一个标签，或直接回车跳过。",
		"Enter exactly %d labels, one per line.\n":                                      "请逐行输入 %d 个标签。\n",
		"Finish with an empty line.":                                                    "用空行结束输入。",
		"Output folder (optional).":                                                     "输出文件夹（可选）。",
		" Leave blank to write next to the program.":                                    " 留空则写到程序所在目录。",
		"Press Enter to continue: ":                                                     "按回车继续：",
		"Select 1 or 2 (or 'yes'/'no'): ":                                               "输入 1 或 2（或 'yes'/'no'）：",
		"Enter a number to choose one candidate.":                                       "输入编号选择一个候选项。",
		"Or enter another keyword to search again.":                                     "或输入其他关键词重新搜索。",
		"Select a species or search again: ":                                            "选择物种或重新搜索：",
		"Selection command (all/none/toggle/on/off/done, plus back/spawn/lobby/exit): ": "输入选择命令（all/none/toggle/on/off/done，外加 back/spawn/lobby/exit）：",
		"Selection command (all/none/toggle/on/off/done, done all, plus back/spawn/lobby/exit): ": "输入选择命令（all/none/toggle/on/off/done，done all，外加 back/spawn/lobby/exit）：",
		"List action (back - return to the table, txt - write the _list file, exit - quit): ":     "列表操作（back - 返回表格，txt - 写出 _list 文件，exit - 退出）：",
		"Selection commands:":                                           "选择命令：",
		"List output actions:":                                          "列表输出操作：",
		"Review the summary and press Enter to continue.":               "查看摘要后按回车继续。",
		"Nucleotide query starts here":                                  "核酸查询从这里开始",
		"Protein query starts here":                                     "蛋白查询从这里开始",
		"Keyword results:":                                              "关键词结果：",
		"Choose 1, 2, 3, or 4 (or 'repeat'/'change'/'mode'/'exit'): ":   "输入 1, 2, 3, 或 4（或 'repeat'/'change'/'mode'/'exit'）：",
		"Choose 1, 2, 3, or 4 (or 'retry'/'skip'/'back'/'exit'): ":      "输入 1, 2, 3, 或 4（或 'retry'/'skip'/'back'/'exit'）：",
		"Select 1, 2, or 3 (or 'retry'/'back'/'exit'): ":                "输入 1, 2, 或 3（或 'retry'/'back'/'exit'）：",
		"Choose 1, 2, or 3 (or 'install'/'back'/'exit'): ":              "输入 1, 2, 或 3（或 'install'/'back'/'exit'）：",
		"Help - species search":                                         "帮助 - 物种搜索",
		"Help - database selection":                                     "帮助 - 数据库选择",
		"Help - mode selection":                                         "帮助 - 模式选择",
		"Help - species selection":                                      "帮助 - 物种选择",
		"Help - BLAST input":                                            "帮助 - BLAST 输入",
		"Help - keyword input":                                          "帮助 - 关键词输入",
		"Help - BLAST row selection":                                    "帮助 - BLAST 结果选择",
		"Help - keyword row selection":                                  "帮助 - 关键词结果选择",
		"Help - export file name":                                       "帮助 - 导出文件名",
		"Help - next action":                                            "帮助 - 下一步操作",
		"Help - fetch error options":                                    "帮助 - 取数错误选项",
		"Help - workflow error options":                                 "帮助 - 工作流错误选项",
		"BLAST plan:":                                                   "BLAST 计划：",
		"  1) choose a database and mode":                               "  1) 选择数据库和模式",
		"  2) pick a species when needed":                               "  2) 在需要时选择物种",
		"  3) paste a sequence, FASTA, URL, or keyword list":            "  3) 粘贴序列、FASTA、URL 或关键词列表",
		"  4) review results, select rows, and export files":            "  4) 查看结果、选择行并导出文件",
		"Detailed run log?":                                             "生成详细运行日志？",
		" 1) yes - write a timestamped log next to the generated files": " 1) yes - 在生成文件旁写入带时间戳的日志",
		" 2) no  - skip the detailed log":                               " 2) no  - 跳过详细日志",
		"yes writes a detailed log after file generation.":              "yes 会在文件生成后写入详细日志。",
		"no skips the extra log.":                                       "no 会跳过额外日志。",
		"Please enter one of: 1, 2, yes, or no.":                        "请输入 1、2、yes 或 no。",
		"Keyword list preview:":                                         "关键词列表预览：",
		"Press Enter to continue.":                                      "按回车继续。",
		"Or use back, spawn, lobby, or exit to navigate away.":          "也可以使用 back、spawn、lobby 或 exit 导航离开。",
		"No species candidates matched %q.\n":                           "没有候选物种匹配 %q。\n",
		"Candidate species for %q:\n":                                   "%q 的候选物种：\n",
		"Language switched for subsequent prompts.":                     "后续提示语言已切换。",
		"Unknown language. Use lang=en, lang=cn, or lang=jp.":           "未知语言。请使用 lang=en、lang=cn 或 lang=jp。",
	},
	Japanese: {
		"Global navigation: back - previous page | spawn - mode selection | lobby - database selection | exit - quit the wizard": "グローバル操作: back - 前のページ | spawn - モード選択 | lobby - データベース選択 | exit - 終了",
		"Database selection:":                                                           "データベース選択:",
		"Mode selection:":                                                               "モード選択:",
		"BLAST program selection:":                                                      "BLAST プログラム選択:",
		"BLAST execution target:":                                                       "BLAST 実行先:",
		"Enter a species keyword: ":                                                     "種キーワードを入力: ",
		"Select 1 or 2 (or 'phytozome'/'lemna'): ":                                      "1 または 2 を入力（または 'phytozome'/'lemna'）: ",
		"Select 1 or 2 (or 'blast'/'keyword'): ":                                        "1 または 2 を入力（または 'blast'/'keyword'）: ",
		"Select a program by number or name (type help for details): ":                  "番号または名前で選択（help で詳細）: ",
		"Select 1 or 2 (or 'server'/'local'): ":                                         "1 または 2 を入力（または 'server'/'local'）: ",
		"Enter one label per search term.":                                              "検索語ごとに 1 つラベルを入力してください。",
		"Use ~ for a blank label.":                                                      "空欄は ~ を使ってください。",
		"Press Enter on the first line to skip all labels.":                             "最初の行を空で Enter すると全ラベルを省略します。",
		"Enter one label for this BLAST query, or press Enter to skip.":                 "この BLAST クエリのラベルを 1 つ入力するか、Enter で省略します。",
		"Finish with an empty line.":                                                    "空行で終了します。",
		"Output folder (optional).":                                                     "出力フォルダ（任意）",
		" Leave blank to write next to the program.":                                    " 空欄ならプログラムと同じ場所に出力します。",
		"Press Enter to continue: ":                                                     "Enter で続行: ",
		"Select 1 or 2 (or 'yes'/'no'): ":                                               "1 または 2 を入力（または 'yes'/'no'）: ",
		"Enter a number to choose one candidate.":                                       "番号を入力して候補を選択します。",
		"Or enter another keyword to search again.":                                     "別のキーワードを入力して再検索できます。",
		"Select a species or search again: ":                                            "種を選択するか再検索: ",
		"Selection command (all/none/toggle/on/off/done, plus back/spawn/lobby/exit): ": "選択コマンド（all/none/toggle/on/off/done、back/spawn/lobby/exit）: ",
		"Selection command (all/none/toggle/on/off/done, done all, plus back/spawn/lobby/exit): ": "選択コマンド（all/none/toggle/on/off/done、done all、back/spawn/lobby/exit）: ",
		"List action (back - return to the table, txt - write the _list file, exit - quit): ":     "リスト操作（back - 表へ戻る、txt - _list ファイルを書き出す、exit - 終了）: ",
		"Selection commands:":                                           "選択コマンド：",
		"List output actions:":                                          "リスト出力操作：",
		"Review the summary and press Enter to continue.":               "要約を確認して Enter で続行してください。",
		"Nucleotide query starts here":                                  "核酸クエリはこちらから",
		"Protein query starts here":                                     "タンパク質クエリはこちらから",
		"Keyword results:":                                              "キーワード結果：",
		"Choose 1, 2, 3, or 4 (or 'repeat'/'change'/'mode'/'exit'): ":   "1, 2, 3, 4 を入力（または 'repeat'/'change'/'mode'/'exit'）: ",
		"Choose 1, 2, 3, or 4 (or 'retry'/'skip'/'back'/'exit'): ":      "1, 2, 3, 4 を入力（または 'retry'/'skip'/'back'/'exit'）: ",
		"Select 1, 2, or 3 (or 'retry'/'back'/'exit'): ":                "1, 2, 3 を入力（または 'retry'/'back'/'exit'）: ",
		"Choose 1, 2, or 3 (or 'install'/'back'/'exit'): ":              "1, 2, 3 を入力（または 'install'/'back'/'exit'）: ",
		"Help - species search":                                         "ヘルプ - 種検索",
		"Help - database selection":                                     "ヘルプ - データベース選択",
		"Help - mode selection":                                         "ヘルプ - モード選択",
		"Help - species selection":                                      "ヘルプ - 種選択",
		"Help - BLAST input":                                            "ヘルプ - BLAST 入力",
		"Help - keyword input":                                          "ヘルプ - キーワード入力",
		"Help - BLAST row selection":                                    "ヘルプ - BLAST 行選択",
		"Help - keyword row selection":                                  "ヘルプ - キーワード行選択",
		"Help - export file name":                                       "ヘルプ - 出力ファイル名",
		"Help - next action":                                            "ヘルプ - 次の操作",
		"Help - fetch error options":                                    "ヘルプ - 取得エラーの選択肢",
		"Help - workflow error options":                                 "ヘルプ - ワークフローエラーの選択肢",
		"BLAST plan:":                                                   "BLAST の流れ：",
		"  1) choose a database and mode":                               "  1) データベースとモードを選ぶ",
		"  2) pick a species when needed":                               "  2) 必要なら種を選ぶ",
		"  3) paste a sequence, FASTA, URL, or keyword list":            "  3) 配列、FASTA、URL、キーワード一覧を貼る",
		"  4) review results, select rows, and export files":            "  4) 結果を確認し、行を選択して出力する",
		"Detailed run log?":                                             "詳細な実行ログを出力しますか？",
		" 1) yes - write a timestamped log next to the generated files": " 1) yes - 生成ファイルの横にタイムスタンプ付きログを出力",
		" 2) no  - skip the detailed log":                               " 2) no  - 詳細ログを出力しない",
		"yes writes a detailed log after file generation.":              "yes でファイル生成後に詳細ログを書き出します。",
		"no skips the extra log.":                                       "no で追加ログを省略します。",
		"Please enter one of: 1, 2, yes, or no.":                        "1、2、yes、no のいずれかを入力してください。",
		"Keyword list preview:":                                         "キーワード一覧プレビュー：",
		"Press Enter to continue.":                                      "Enter で続行してください。",
		"Or use back, spawn, lobby, or exit to navigate away.":          "back、spawn、lobby、exit でも移動できます。",
		"No species candidates matched %q.\n":                           "%q に一致する候補種がありません。\n",
		"Candidate species for %q:\n":                                   "%q の候補種：\n",
		"Please enter a number or another keyword.":                     "番号または別のキーワードを入力してください。",
		"Language switched for subsequent prompts.":                     "以降のプロンプト言語を切り替えました。",
		"Unknown language. Use lang=en, lang=cn, or lang=jp.":           "不明な言語です。lang=en、lang=cn、lang=jp を使用してください。",
	},
}

func Text(lang Language, english string) string {
	if english == "" {
		return ""
	}
	if lang == English {
		return english
	}
	if table, ok := translations[lang]; ok {
		if translated, ok := table[english]; ok && translated != "" {
			return translated
		}
	}
	return english
}

func Sprintf(lang Language, english string, args ...any) string {
	return fmt.Sprintf(Text(lang, english), args...)
}
