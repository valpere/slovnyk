# Рев’ю M1-01–04 та CI — 2026-10-24

## Знахідки

**P2 — перевіряти staged та committed whitespace.** Початковий `Makefile`
викликав лише `git diff --check`: перед commit він не помічав уже staged patch,
а на clean CI checkout завжди перевіряв порожній working diff. Це дозволяло
повідомити PASS без перевірки patch, який фактично комітиться.
Виправлено через [check-patch.sh](../../../minions/check-patch.sh): staged і
unstaged diff локально, base→HEAD у CI, HEAD для першого push/manual.
Historical assessments виключені явно: їхні bytes/hashes зберігаються як докази.
У disposable Git repo відтворено пропуск staged trailing space; після
виправлення такий patch відхиляється, clean і historical evidence проходять.

Інших підтверджених дефектів у переглянутій реалізації не знайдено. Це
послідовне рев’ю тим самим агентом; незалежне або багатомодельне рев’ю
цього коду не проводилося. Виправлений patch і новий workflow переглянуто повторно.

## Межі й точна версія

Base: `ac340046c0910116213e29948b0f6c72c885f693`.
Scope: working/staged/untracked M1-01–04 application code, embedded SQL,
synthetic fixtures, Make/minions/Compose, активні contracts/docs і новий CI.
Точний вміст зафіксовано у [reviewed-inputs.json](reviewed-inputs.json)
через SHA-256 кожного вхідного файла. Цей звіт не входить у власний hash inventory.
Історичні raw council outputs/snapshots не переоцінювалися як нова реалізація;
їх збережено без переписування. Review skill:
`/home/val/.codex/skills/code-review/SKILL.md`.

Розглянуто admission/parser bounds, Unicode/raw identity, hash match до merge,
per-dataset lock, retry і transaction atomicity, ledger/origin reconciliation,
terminal states, SQL privileges, HTTP lifecycle/privacy, CLI errors і документацію.
Повний Go import corpus та production capacity не перевірялися. Verify/ready/
publish/rollback/search/UI — заплановані M1-05–08, не оголошені готовими.

## CI та перевірки

GitHub Actions дозволені для private `valpere/slovnyk`; workflow раніше не було.
[CI](../../../.github/workflows/ci.yml) додає push/PR/manual triggers і дві jobs:
Go check/unit/race/HTTP smoke та disposable PostgreSQL integration suites.
Runner — Ubuntu 24.04, Go із `.go-version`, checkout/setup-go pinned за SHA,
token `contents: read`, без production secrets чи deploy. Browser не потрібний.
Branch protection та автоматичний merge не налаштовувалися.

`actionlint v1.7.12` — PASS. Локальні check/unit/race/HTTP smoke та PostgreSQL
schema/importer/CLI suites — PASS, попередні результати M1-04 у відповідному
[звіті](../2026-10-24-staged-import/REPORT.md). Нова staged-whitespace перевірка
окремо відтворена у тимчасовому Git repo. Runtime evidence — `tmp/ci-review/`.
Завантаження workflow у робочу гілку та фактичний remote CI виконуються після
створення комітів; цей pre-commit звіт не заявляє remote PASS наперед.

Пряма вказівка користувача: надалі після кожного завершеного етапу обов'язкові
рев’ю, належні перевірки, локальний commit і CI для опублікованих змін.
`AGENTS.md` не змінено; правило збережено у [DECISIONS](../../DECISIONS.md).
