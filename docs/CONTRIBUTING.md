# Review → PR → CI

Перед публікацією PR перевірте локальний diff від його base, контракти й
документацію. Зафіксуйте scope (base/head або hashes), підтверджені findings,
виправлення й обмеження у звіті. Чесно позначайте self-review; він не є
незалежним рев’ю. Виконайте належні локальні перевірки до commit.

Відкрийте PR у `main` із поведінкою, пов’язаними вимогами, посиланням на review,
командами/результатами та залишковими обмеженнями. Використовуйте
[шаблон PR](../.github/pull_request_template.md). Значущі подальші зміни знову
перегляньте перед push, оновіть review evidence та опис PR.

[CI](../.github/workflows/ci.yml) автоматично запускається на `pull_request`:
Go checks/unit/race/HTTP smoke та ізольовані PostgreSQL integration.
Обидві jobs мають пройти для останньої версії PR до merge. Перевіряйте event,
head SHA й conclusion через `gh pr checks` / `gh run view`; зелений push run
не замінює PR merge-revision run. Злиття й deploy потребують окремої вказівки.
Branch protection цим документом не налаштовується.

Перший PR цього процесу — [#1](https://github.com/valpere/slovnyk/pull/1).
Його перший CI виявив CRLF/bare-CR parser fixtures у повному diff від main.
Правило [.gitattributes](../.gitattributes) дозволяє CR-at-EOL лише для двох
точних fixtures та вимикає їхню text conversion. Їхні bytes/hashes незмінні;
інші файли проходять звичайну whitespace-перевірку.
