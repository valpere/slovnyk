# Старі URL: історична довідка

**Статус: сумісність скасована 2026-10-23.** Користувач підтвердив, що старі
пошукові та download-посилання підтримувати не потрібно (OQ-04 закрито).
Legacy adapter, mapping параметрів і redirects не входять до обсягу нового
сервісу. UC-07, FR-12 та AT-12 скасовані; ID не перевикористовуються.
Це не скасовує перенесення словникових даних або нових завантажень.

Observed baseline: старий HEAD `94640dc6fe2fde643af3ee746254e0d60990a0ce`.
Наведені факти пояснюють стару реалізацію, а не задають нову поведінку.
Контракт нового пошуку — [LOOKUP_CONTRACT](LOOKUP_CONTRACT.md).

## Перевірені факти

- Sitemap містить `/fcgi-bin/dic.fcgi?iw=Harvest&hn=sel&il=en_us&ol=uk_ua&ul=en-us`.
- `cgi/dic.fcgi:190–225` читає `hn,il,ol,iw,ul`; параметра `hl` немає.
- Missing hn → hom; unknown hn у старому теж hom. Missing il → ul (:318),
  missing ol → en-us (`cfg/dic.conf:58`); UI fallback через Accept-Language.
- `hn=pre` викликає той самий lookup (:396–399), але старий результат — HTML
  з PRE (`pre.html:1–3`, config MIME text/html), не text/plain.
- Sanitizer (:169–176) видаляє `<>&%"'`, міняє underscores/whitespace; мовні
  underscores у sitemap суперечать цьому parser. Same-locale пошук блокується (:352).
- `dwn.html:176` посилається на `/csv/{FILE}`; `loc.html:37,49` —
  `/loc/slovnyk.{code}.po`. Додаткові aliases/domains/rewrite rules невідомі.

## Попередня пропозиція

Історичну пропозицію routes, mapping і приймальної матриці збережено
в [snapshot від 2026-10-21](_assessments/2026-10-21-contracts/final-snapshot/LEGACY_COMPATIBILITY.md).
Вона не є чинним контрактом і не відновлює вимогу сумісності.
