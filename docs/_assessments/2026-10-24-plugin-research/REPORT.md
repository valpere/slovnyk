# Корисні плагіни для Slovnyk — 2026-10-24

## Джерела та стан

Переглянуто локальний `~/wrk/github_repos/orgs/anthropics/claude-plugins-official`,
commit `58267944` від 2026-10-23: marketplace entries, README, manifest та
MCP config кандидатів. Каталог не оновлювався і не модифікувався.
Додатково перевірено [офіційну документацію CLI plugins](https://learn.chatgpt.com/docs/plugins?surface=cli),
[CLI commands](https://learn.chatgpt.com/docs/developer-commands),
[формат плагінів і локальних каталогів](https://developers.openai.com/plugins/build/plugins),
локальний `codex plugin --help`, installed list і пошук доступних плагінів.

Жодного плагіна чи marketplace не встановлено/підключено; hooks не запускалися.
CLI installed list містить openai-templates, sites, plugin-management, work-pets.
Окремі review skills уже доступні в поточній сесії; наявність skill не означає
встановлення однойменного плагіна. Сирі локальні списки — `tmp/plugin-research/`.
Розробка M1 залишається на паузі, Oracle не використовувався.

## Кандидати та пріоритет

| Кандидат | Користь для проєкту | Стан / рекомендація |
|---|---|---|
| Context7 | Документація та приклади конкретних версій залежностей; корисно при Unicode normalization і storage libraries | Знайдений у доступному каталозі, installed=false; запропоновано для встановлення. Найкорисніше нове доповнення зараз, хоча на етапі stdlib потреба обмежена |
| frontend-design | Типографіка, композиція та сучасний UI словника | Є локальний skill-based Claude plugin; кандидат для адаптації до Codex перед M1-07. Застосовувати до погодженого мінімалістичного search UI та no-JS вимог |
| Playwright | Browser/no-JS сценарії, мовні налаштування, accessibility-oriented перевірки, screenshots | Є локальний MCP plugin; доцільно перед M1-07. Конкретну доступність у Codex не підтверджено; Node/npx і browser setup перевірити окремо |
| pr-review-toolkit | Окремі перевірки comments, tests, silent failures, types та коду | Наявні code-review, comment-analyzer, pr-test-analyzer, silent-failure-hunter skills уже покривають значну частину. Повторне встановлення зараз не потрібне |
| gopls-lsp | Go navigation, references, refactoring | Локальний plugin призначений для Claude Code й потребує gopls executable. Підтримку його lspServers у Codex не підтверджено; не встановлювати як гарантовано сумісний Codex plugin |
| GitHub | Issues, PR, CI і code review у GitHub | Знайдений у доступному каталозі, installed=false. Корисний із появою PR workflow; зараз локальна робота не потребує інтеграції |

Джерела кандидатів у локальному каталозі: `plugins/frontend-design/`,
`plugins/pr-review-toolkit/`, `plugins/gopls-lsp/`,
`external_plugins/context7/`, `external_plugins/playwright/`, `external_plugins/github/`.

## Що врахувати перед установленням

Локальний Context7 config використовує hosted HTTP MCP. Запити до документації
передаватимуться зовнішньому сервісу; словниковий корпус для цього не потрібен.
Нативний Context7 listing знайдено окремо: це не доказ тотожності всіх
можливостей та налаштувань із Claude wrapper. Реальний MCP виклик не перевірявся.

Локальний Playwright config запускає `npx @playwright/mcp@latest`.
Для відтворюваної роботи варто зафіксувати перевірену версію перед використанням.
GitHub Claude config очікує `GITHUB_PERSONAL_ACCESS_TOKEN`; умови native
GitHub plugin перевіряти окремо, не переносити credentials між ними автоматично.

CLI має `/plugins`; після встановлення bundled skills стають доступними в новій
сесії. Локальний marketplace підтримується командами CLI, але це не гарантує
перенесення кожного Claude-specific agent, command, hook чи LSP component.
Їхню сумісність потрібно перевіряти по складових.

`feature-dev` зараз дублює наявний M1 workflow, `skill-creator` уже доступний.
`hookify` і `security-guidance` містять автоматичні hooks; їх не обрано до
початкового набору. Поточні явні Make/minions checks залишаються основними.
