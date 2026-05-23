---
name: xhs-search
description: Search Xiaohongshu posts by keyword and return structured data via XHR interception
allowed-tools: ai-browser
user-invocable: false
---

# XHS Search

Search Xiaohongshu (Little Red Book) for posts matching a keyword. Returns structured JSON with post data extracted via XHR API interception — no DOM scraping.

## When to use

When you need to search Xiaohongshu for posts by keyword. This skill handles login detection, search execution, sorting, time filtering, and pagination automatically.

## Prerequisites

- Page must be navigated to the XHS search URL **before** calling `browser_run`
- User must be logged in to Xiaohongshu in the browser session

## Steps

1. `browser_navigate` to:
   ```
   https://www.xiaohongshu.com/search_result?keyword={keyword}&source=web_search_result_notes
   ```
   Replace `{keyword}` with the URL-encoded search term.

2. Wait for the page to load (use `browser_wait_for` or a short delay).

3. `browser_run` with this skill's `index.js`:
   ```
   browser_run({
     file: "{path_to_this_skill}/index.js",
     params: {
       sort_by: "time_descending",
       time_range: "一周内",
       pages: 2
     }
   })
   ```

## Params

| Param | Type | Required | Default | Description |
|---|---|---|---|---|
| sort_by | string | No | `"general"` | Sort order: `general`, `time_descending`, `popularity_descending`, `comment_descending`, `collect_descending` |
| time_range | string | No | `"不限"` | Time filter: `不限`, `一天内`, `一周内`, `半年内` |
| pages | number | No | `1` | Number of pages to fetch (1–5). Each page returns ~20 posts. |

## Returns

```json
{
  "success": true,
  "logged_in": true,
  "user": {
    "user_id": "...",
    "nickname": "..."
  },
  "posts": [
    {
      "id": "post_id",
      "title": "Post title",
      "author": "Author nickname",
      "liked_count": 1200,
      "collected_count": 350,
      "comment_count": 89,
      "type": "normal",
      "link": "https://www.xiaohongshu.com/explore/post_id"
    }
  ],
  "total": 40,
  "has_more": true
}
```

## Error cases

| Condition | Return |
|---|---|
| Not logged in | `{ "success": false, "logged_in": false, "error": "Not logged in to Xiaohongshu" }` |
| No search results | `{ "success": true, "logged_in": true, "posts": [], "total": 0, "has_more": false }` |
| API timeout | `{ "success": false, "error": "Search API did not respond within timeout" }` |
