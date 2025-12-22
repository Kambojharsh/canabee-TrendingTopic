# get_trending_topics Lambda

## Endpoint
```
GET /trending-topics?latitude=12.9716&longitude=77.5946&radius=10
```

## Input (Query Parameters)
- `latitude` (required): Latitude coordinate (-90 to 90)
- `longitude` (required): Longitude coordinate (-180 to 180)
- `radius` (optional): Search radius in miles (default: 10)

## Output
```json
{
  "trending_tags": [
    {"tag": "pain_relief", "count": 34},
    {"tag": "headache", "count": 27}
  ]
}
```

## Database Table
Uses the `sessions` table with columns:
- `tag`: Session tag
- `latitude`: Session latitude
- `longitude`: Session longitude

## Logic
1. Finds all sessions within given radius using Haversine formula
2. Counts frequency of each tag
3. Returns top 5 tags sorted by count (descending)
4. Returns empty array if no tags found