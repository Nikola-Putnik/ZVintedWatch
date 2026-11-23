# Cash Converters module

Source: [`internal/provider/cashconverters`](../../internal/provider/cashconverters). This page
documents how the module talks to [Cash Converters Belgium](https://www.cashconverters.be), for
maintainers. The online shop runs on **PrestaShop**; everything specific to it lives in this
package.

## Requests

| Step | Request | Notes |
|---|---|---|
| Search | `GET https://www.cashconverters.be/<page>?…&order=product.date_add.desc&resultsPerPage=36&page=1&ajax=1` | `<page>` is the search page (`/fr/recherche?controller=search&s=…`, `/nl/zoeken?…`) or a category page (`/fr/134-informatique`). With `ajax=1` and the `X-Requested-With: XMLHttpRequest` header, PrestaShop answers with JSON instead of HTML. |
| Store | `GET <product page URL>` | Only for listings that **matched an alert** (`provider.Enricher`). The page is streamed and reading stops once the store block is received (~105 KB of ~146 KB). |

No session or account is needed. The PrestaShop cookies set by the website are kept in the
lane's cookie jar like a browser would.

### Sorting

The website only offers relevance, name and price sorting in its menu, but PrestaShop also
accepts `order=product.date_add.desc`, which returns the **newest items first**. This is what
makes one small request per refresh sufficient. Note that the website's `robots.txt` asks
indexing crawlers not to fetch URLs containing `order=`; ZVintedWatch is not a crawler (a few
paced requests, like a visitor refreshing a page), and this choice is deliberate.

## Search parameters

| Pasted address | Kept |
|---|---|
| `/fr/recherche?controller=search&s=…`, `/nl/zoeken?…` | language, `s` (lowercased, spaces collapsed) |
| `/fr/<id>-<slug>` (category) | the category path |
| `q=Magasins-WAVRE-ETTERBEEK/Prix-€-100-500` | the facet filters (stores, price), unchanged |
| `order`, `page`, `resultsPerPage`, `ajax`, … | dropped (set by the module) |

Plain keywords search the French website. Product pages (`…/123-name.html`) are rejected with a
helpful message. The page path is stored in the query under the internal `_path` parameter so
that two different pages never share a key; it is removed before the request is sent.

## Listing fields

| JSON field | Listing |
|---|---|
| `id_product` | `ID` and `Sequence` (ids are sequential) |
| `name` | `Title` (HTML entities decoded) |
| `description_short` | `Description` (tags removed) — it is the same text as the product page, so `DescriptionInSearch` is `true` |
| `price_amount` | `Price` (EUR, taxes included) |
| `has_discount`, `regular_price_amount` | `Condition` = `−25% (was 80.00 €)` when discounted |
| `category_name` | `Category` |
| `cover.bySize.large_default.url` | `PhotoURL` (JPEG) |
| `url` | `URL` |

`manufacturer_name` is always “Cash Converters” and is ignored.

## Store

The product page contains:

```html
<div class="product-shop">
  <h2>Magasin</h2>              <!-- "Winkel" on Dutch pages -->
  <div class="shop-infos">
    <p>L'article est disponible dans le magasin: Mouscron</p>
    <p>Rue de la Liesse 96, 7700 Mouscron</p>
    <p>Numéro de téléphone: …</p>
```

`Location` becomes `Mouscron – Rue de la Liesse 96, 7700 Mouscron`. It appears in Telegram
messages (📍) and in the match history.

## Limits and defaults

- Minimum refresh interval 30 s, 2 min recommended: it is a small website without a CDN.
- 36 newest items per refresh. A search receiving more than 36 new items between two refreshes
  would miss some; the activity console warns when every item of a refresh is new.
- Only the Belgian website is supported. Cash Converters sites of other countries use different
  platforms and would be separate modules.

## When the website changes

1. Reproduce with the debug commands (the module is detected from the URL):
   ```sh
   zvintedwatch debug search "https://www.cashconverters.be/fr/recherche?controller=search&s=mac+mini"
   zvintedwatch debug enrich "https://www.cashconverters.be/fr/<category>/<id>-<name>.html"
   ```
2. Check the JSON in a browser: open the search address with `&ajax=1` added.
3. Update `search.go` (parameters, JSON fields), `query.go` (address parsing) or `item.go`
   (store block), and the synthetic fixtures in `cashconverters_test.go`.
