# Lambda Functions

This directory contains AWS Lambda functions for the Cannabee application.

## get_trending_topics

A Lambda function that retrieves trending topics (tags) based on location and radius.

### Input (GET Request)
```
GET /trending-topics?latitude=12.9716&longitude=77.5946&radius=10
```

**Query Parameters:**
- `latitude` (required): Latitude coordinate (-90 to 90)
- `longitude` (required): Longitude coordinate (-180 to 180)
- `radius` (optional): Search radius in miles (default: 10)

### Output
```json
{
  "trending_tags": [
    {"tag": "pain_relief", "count": 34},
    {"tag": "headache", "count": 27},
    {"tag": "fever", "count": 21},
    {"tag": "cold", "count": 18},
    {"tag": "sore_throat", "count": 15}
  ]
}
```

### Testing with Real Database

1. **Set up environment variables:**
   ```bash
   # Option 1: Copy and edit the example file
   cp env-example.txt .env
   # Edit .env with your actual database credentials

   # Option 2: Set environment variables directly
   export DB_HOST=your-host
   export DB_USER=your-username
   export DB_PASSWORD=your-password
   export DB_NAME=your-database
   export DB_PORT=3306
   ```

2. **Configure test parameters:**
   Edit `test_real_db.py` and modify these variables:
   ```python
   TEST_LATITUDE = 12.9716   # Your test latitude
   TEST_LONGITUDE = 77.5946  # Your test longitude
   TEST_RADIUS = 50          # Search radius in miles
   ```

3. **Run the test:**
   ```bash
   cd lambda
   source venv/bin/activate  # or however you activate your virtual environment
   python test_real_db.py
   ```

### Local Development

1. **Setup virtual environment:**
   ```bash
   python3 -m venv venv
   source venv/bin/activate
   pip install -r requirements.txt
   ```

2. **Run unit tests:**
   ```bash
   python test_trending_topics.py
   ```

3. **Run real database test:**
   ```bash
   python test_real_db.py
   ```

### Files

- `get_trending_topics.py` - Main Lambda function
- `test_trending_topics.py` - Unit tests with mocked database
- `test_real_db.py` - Integration test with real database
- `requirements.txt` - Python dependencies
- `env-example.txt` - Environment variables template
- `.gitignore` - Git ignore rules