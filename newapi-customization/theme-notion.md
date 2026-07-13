# theme-notion — `[data-theme-preset='notion']` 规范与落点

> 规范出处 PLAN.md §5.1（量化自 xju-feiyue）；落地文件见下。预设已设为**默认**。

## 落点清单（重放时逐项核对）

| # | 文件 | 改动 |
|---|---|---|
| 1 | `src/styles/theme-presets.css` | 追加 `[data-theme-preset='notion']` + `.dark [data-theme-preset='notion']` 完整块（anthropic 块之后、Font axis 之前）；并把 `notion` 加进 **semantic surface bridge 的两处 `:not()` 豁免链**（浅色/深色各一处）——不豁免的话暖灰表面会被蓝色 primary 派生色覆盖 |
| 2 | `src/lib/theme-customization.ts` | `THEME_PRESETS` 加 `{ value:'notion', name:'Notion', swatches:[...] }`（类型/校验/设置面板全自动派生）；`PRESET_DEFAULT_FONT` 加 `notion:'sans'`；`DEFAULT_THEME_CUSTOMIZATION.preset` 改 `'notion'`（默认皮肤） |
| 3 | `src/i18n/locales/{en,zh,zh-TW,fr,ru,ja,vi}.json` | 各加 `"preset.notion": "Notion"` |

## 色板映射（hex → oklch 近似）

**浅色**：

| 语义 | Hex | oklch | 槽位 |
|---|---|---|---|
| 背景 | `#ffffff` | `1 0 0` | `--background` `--card` `--popover` |
| 暖米次背景 | `#f7f6f3` | `0.972 0.004 95` | `--muted` `--sidebar` |
| hover 灰 | `#f1f1ef` | `0.956 0.003 100` | `--accent` `--secondary`（hover=背景变浅，非阴影加深） |
| 暖黑正文 | `#37352f` | `0.325 0.009 96` | `--foreground` |
| 次要字 | `#787774` | `0.565 0.006 95` | `--muted-foreground` |
| 弱字 | `#9b9a97` | `0.68 0.005 100` | `--neutral` |
| 极浅分割线 | `#edece9` | `0.938 0.004 95` | `--border` `--sidebar-border` |
| 强分割线 | `#dcdad4` | `0.888 0.005 95` | `--input` |
| 链接蓝 | `#2383e2` | `0.6 0.152 252` | `--primary` `--ring` `--info` `--chart-1` |
| 绿/橙/紫/红 | `#0f7b6c` `#d9730d` `#9065b0` `#e03e3e` | 见预设块 | `--success` `--warning` `--chart-2..5` `--destructive` |

**深色**（Notion 深色系）：背景 `#191919`→`0.225 0 0`，卡片 `#202020`，文字 `#d3d3d1`，边框 `oklch(1 0 0 / 9%)`，主色升明为 `0.68 0.11 245`（≈`#529cca`）。

## 圆角 / 阴影 / 字体

- `--radius: 0.5rem`（8px）——全站 `--radius-sm/md/lg/...` 由 `calc` 派生，一处即改全站（`theme.css:87-93`）。
- **不加阴影变量**：该代码库无 `--shadow-*` 主题槽，阴影是组件级 Tailwind 类；Notion 观感靠 border 分层已由色板实现。
- 字体：`--font-body` 保持 sans（`PRESET_DEFAULT_FONT.notion='sans'`）；**标题/大数字组件级加 `font-serif`**（现有 `--font-serif` 栈已含 Source Serif 4 + 完整中文宋体链，未新增字体包）。已应用于首页 hero h1、各 section h2、Stats 大数字（`font-serif font-semibold tabular-nums`）。
- 滚动条：沿用全局既有 `* { scrollbar-width: thin; scrollbar-color: var(--border) transparent }`（`index.css`），预设色板生效后即为 Notion 细滚动条，无需新增工具类。
