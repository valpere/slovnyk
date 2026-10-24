# Перевірка ресурсів локалізації — 2026-10-23

## Метод і межі

Лише читання `../slovnyk1.0/`. У `data/loc/` знайдено 31 PO-файл,
у `cgi/share/locales/` — 13; усі 13 байтово збігаються з однойменними
файлами в `data/loc/`. Старі ресурси не змінювалися.
Для кожного файла виконано `LC_ALL=C msgfmt --statistics -o /dev/null <path>`.
Exit 0 означає успішну компіляцію цим інструментом, а не перевірену якість
перекладу, безпечність HTML, повноту нового UI чи production-ready локаль.

`cgi/dic.fcgi:217–227` обирає явний `ul`, потім результат обробки
`HTTP_ACCEPT_LANGUAGE`, потім configured default. `:106–119` готує повні
й короткі коди. `cgi/etc/cfg/dic.conf:58` задає `en-us` за замовчуванням.
Це спостереження коду, не перевірка поведінки старого production або точного
алгоритму language negotiation залежності Wono.

## Результати

| Файл у `data/loc/` | Exit | Діагностика msgfmt |
|---|---|---|
| `slovnyk.be-by.po` | 0 | 66 translated messages, 4 untranslated messages. |
| `slovnyk.bg-bg.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.cs-cz.po` | 1 | ../slovnyk1.0/data/loc/slovnyk.cs-cz.po:192:7: syntax error; ../slovnyk1.0/data/loc/slovnyk.cs-cz.po:199: missing 'msgstr' section; msgfmt: found 2 fatal errors |
| `slovnyk.da-dk.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.de-de.po` | 0 | 68 translated messages, 2 untranslated messages. |
| `slovnyk.el-gr.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.en-gb.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.eo-xx.po` | 0 | 62 translated messages, 8 untranslated messages. |
| `slovnyk.es-es.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.et-ee.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.fi-fi.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.fr-fr.po` | 0 | 68 translated messages, 2 untranslated messages. |
| `slovnyk.hr-hr.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.hu-hu.po` | 0 | 68 translated messages, 2 untranslated messages. |
| `slovnyk.is-is.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.it-it.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.la-va.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.lt-lt.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.lv-lv.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.mk-mk.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.nl-nl.po` | 0 | 67 translated messages, 3 untranslated messages. |
| `slovnyk.no-no.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.pl-pl.po` | 0 | 57 translated messages, 13 untranslated messages. |
| `slovnyk.pt-pt.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.ro-ro.po` | 0 | 68 translated messages, 2 untranslated messages. |
| `slovnyk.ru-ru.po` | 0 | 70 translated messages. |
| `slovnyk.sk-sk.po` | 0 | 67 translated messages, 3 untranslated messages. |
| `slovnyk.sl-si.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.sr-rs.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.sv-se.po` | 0 | 0 translated messages, 70 untranslated messages. |
| `slovnyk.uk-ua.po` | 0 | 70 translated messages. |

Чеський каталог має некоректні секції `msgid`/`msgstr` біля рядків 192–199;
потрібне виправлення в новому проєкті. Англійський `en-gb` містить порожні
`msgstr`: англійські `msgid` самі є текстами, тому нуль translated messages
не означає відсутності англомовного UI. Інші порожні каталоги — заготовки.

## Наслідки для нового сервісу

Підтверджено користувачем: зберегти автоматичне підлаштування UI під мову
браузера; використовувати наявні переклади як джерело; сучасний механізм
локалізації дозволений; відсутні переклади й нові фрази доперекладати ШІ.
Новий UI має власний каталог повідомлень: старі 70 повідомлень не є його
вичерпним контрактом. Зіставити актуальні фрази, зберегти придатні переклади,
додати нові й пропущені. Перевіряти placeholders, HTML escaping і відображення
в макетах; машинно згенерований переклад не називати людськи вичитаним.
Конкретний i18n-пакет/формат і остаточна fallback-мова ще не обрані.
