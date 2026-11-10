# Кандидати для першого запуску

Перевірено 2026-11-10 за публічними сторінками провайдерів. Це порівняння,
**власник ще не обрав сервер**. Замовлень, витрат, звернень до підтримки,
перевірок tenancy або підключень до зайнятого Oracle не було.

## Рекомендація

Першим кандидатом для підтвердження ціни/наявності та подальшого приймання
пропоную **netcup VPS Lite 3 G12.5s**: advertised 8 vCore, 16 GB RAM,
320 GB SSD. Ціна сторінки — **€16.66/місяць із 19% VAT**, IPv4+IPv6,
setup €0, мінімальний строк і billing period — 2 місяці. За цією конфігурацією
перший період арифметично **€33.32**; це не checkout quote для власника.
Податок залежить від billing country; знижок, валютної конвертації й невідомих
додаткових опцій у розрахунку немає.
[Офіційний тариф](https://www.netcup.com/en/server/vps/vps-lite-3-g12.5s-iv-2m).

320 GB = приблизно 298.0 GiB до filesystem overhead. За місткістю це кандидат
для 200 GiB data + 20 GiB ОС із [профілю](SERVER_PROFILE.md); actual usable bytes
та MemTotal ще не виміряні. 8 advertised vCore не доводять гарантований CPU
час або продуктивність PostgreSQL. Геолокація автоматична серед Nuremberg,
Vienna, Amsterdam. Порт 1 Gbps; середній traffic понад 100 Mbps за 24 h може
спричинити тимчасове обмеження до 100 Mbps. Розширення local block storage
заявлене, але його ціну не включено. Сторінка попереджає про можливі затримки
provisioning; наявність до конкретної дати не підтверджена. Джерело — той самий тариф.

Це **кандидат у перевіреній вибірці**, не твердження про найдешевший VPS на ринку.
Серверного варіанта близько $10, який без змін покриває поточний профіль,
цією перевіркою не підтверджено. Автоматично збільшувати бюджет або зменшувати
резерви не будемо; рішення про ціну й строк залишається за власником.

## Інші перевірені варіанти

| Варіант | Публічна пропозиція | Висновок для поточного профілю |
|---|---|---|
| Oracle Always Free A1 | 1500 OCPU h / 9000 GB h на місяць; для Always Free tenancy еквівалент 2 OCPU/12 GB; 200 GB boot+block сумарно | CPU/RAM і 200 GiB data + ОС не покриває; доступна tenancy quota невідома |
| Hetzner Cost-Optimized | Публічна сторінка показує `Currently not available` | Не планувати запуск на непідтвердженій наявності; current checkout price не підтверджено |
| OVHcloud VPS-4, сторінка «VPS 2027» | Від $23.37/місяць, 8 vCore/24 GB/200 GB NVMe | 200 GB ≈186.3 GiB разом із ОС: потрібен додатковий диск/інший тариф і повна ціна |

[Oracle Always Free](https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier_topic-Always_Free_Resources.htm)
також вказує minimum boot 47 GB, home-region capacity constraints та можливість
reclaim idle instances. Старі згадки 4 OCPU/24 GB не є актуальною гарантією;
для існуючого account потрібна окрема перевірка фактичних quotas/billing.
Платний A1 із достатнім диском можна розглянути окремо за account quote;
без нього ціну й доступність не припускаємо. Поточний Micro лишається зайнятим.

Джерела інших рядків: [Hetzner Cloud](https://www.hetzner.com/cloud/),
[OVHcloud VPS](https://www.ovhcloud.com/en/vps/). Назву «VPS 2027» відтворено
зі сторінки, не як дату перевірки. OVH показує ціну «від», не повний checkout;
строк, регіон, податки й storage option потрібно підтвердити. Вбудована provider
backup-копія не замінює узгоджений logical bundle із файлами та secrets.

## Що потрібно перед оплатою й прийманням

1. Власник обирає провайдера, допустиму суму за перший/наступний період,
   строк контракту та регіон. Перед оплатою звірити cancellation/renewal умови,
   VAT, IPv4, трафік/egress, storage й остаточну суму. Не купувати довгий контракт
   лише на підставі advertised specs.
2. Підтвердити доступність, CPU sharing/fair-use, вид диска та можливість
   потрібного partition/mount layout. На виданому хості зафіксувати `lscpu`,
   `free -b`, `lsblk -b`, `df -B1`, actual free bytes та image architecture.
   Публічний опис RAM у GB не підмінює measured GiB.
3. За [deployment runbook](../deploy/README.md) відновити bundle й виконати
   погоджений [target load + off-host restore <=24 h](LAUNCH_PLAN.md).
   Придатність визначають actual storage/CPU/RAM, SLO й recovery, а не тарифна назва.
4. Зберегти receipt, перевірити Telegram, independent outage watchdog і secret
   escrow; завершити мовне приймання. Лише потім — рішення про відкриття доменів.

Backup лишається на локальному комп’ютері після оновлень, без daily jobs і
автоматичного видалення. У цій оцінці його новий cloud storage не купується;
локальна місткість, uptime, інтернет і збереження копій потребують перевірки
за [backup policy](BACKUP_RESTORE.md#політика-першого-read-only-запуску-2026-11-10).
