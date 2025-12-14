import json
import pymysql
import os
from collections import Counter

# Database settings
db_host = os.environ.get('DB_HOST', '')
db_user = os.environ.get('DB_USER', '')
db_password = os.environ.get('DB_PASSWORD', '')
db_name = os.environ.get('DB_NAME', '')
DB_PORT = os.environ.get('DB_PORT', '3306')

# CORS headers
CORS_HEADERS = {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Credentials": True,
    "Access-Control-Allow-Headers": "Origin,Content-Type,X-Amz-Date,Authorization,X-Api-Key,X-Amz-Security-Token,locale",
    "Access-Control-Allow-Methods": "GET, OPTIONS"
}


def get_trending_topics(event, context):
    print("Received event: " + json.dumps(event, indent=2))
    
    # Parse input from query string parameters
    try:
        params = event.get("queryStringParameters") or {}
        latitude = float(params['latitude'])
        longitude = float(params['longitude'])
        radius = float(params.get('radius', 10))  # Default radius: 10 miles
    except (KeyError, TypeError, ValueError) as e:
        print("Error parsing input data: " + str(e))
        return {
            'statusCode': 400,
            'headers': CORS_HEADERS,
            'body': json.dumps({
                'error': 'Invalid input data. Required query parameters: latitude, longitude. Optional: radius',
                'status': False
            })
        }
    
    # Validate coordinates
    if not (-90 <= latitude <= 90):
        return {
            'statusCode': 400,
            'headers': CORS_HEADERS,
            'body': json.dumps({
                'error': 'Invalid latitude. Must be between -90 and 90.',
                'status': False
            })
        }
    
    if not (-180 <= longitude <= 180):
        return {
            'statusCode': 400,
            'headers': CORS_HEADERS,
            'body': json.dumps({
                'error': 'Invalid longitude. Must be between -180 and 180.',
                'status': False
            })
        }
    
    if radius <= 0:
        return {
            'statusCode': 400,
            'headers': CORS_HEADERS,
            'body': json.dumps({
                'error': 'Invalid radius. Must be greater than 0.',
                'status': False
            })
        }
    
    # Connect to the database
    try:
        connection = pymysql.connect(
            host=db_host,
            user=db_user,
            password=db_password,
            database=db_name,
            port=int(DB_PORT),
            cursorclass=pymysql.cursors.DictCursor
        )
        print("Database connection established")
    except Exception as e:
        print("Error connecting to database: " + str(e))
        return {
            'statusCode': 500,
            'headers': CORS_HEADERS,
            'body': json.dumps({
                'error': 'Database connection failed',
                'status': False
            })
        }
    
    try:
        with connection.cursor() as cursor:
            # Query to find sessions within the given radius using Haversine formula
            # Earth's radius in miles: 3959
            # The formula calculates the great-circle distance between two points
            sql = """
                SELECT tag FROM sessions
                WHERE tag IS NOT NULL 
                  AND tag != ''
                  AND latitude IS NOT NULL 
                  AND longitude IS NOT NULL
                  AND (
                    3959 * ACOS(
                        COS(RADIANS(%s)) * COS(RADIANS(latitude)) *
                        COS(RADIANS(longitude) - RADIANS(%s)) +
                        SIN(RADIANS(%s)) * SIN(RADIANS(latitude))
                    )
                  ) <= %s
            """
            cursor.execute(sql, (latitude, longitude, latitude, radius))
            results = cursor.fetchall()
            print(f"Found {len(results)} sessions within {radius} miles")
        
        # Collect all tags and count their frequency
        tags = [row['tag'] for row in results if row['tag']]
        tag_counts = Counter(tags)
        
        # Get top 5 unique tags sorted by count (descending)
        top_tags = tag_counts.most_common(5)
        
        trending_tags = [
            {"tag": tag, "count": count}
            for tag, count in top_tags
        ]
        
        return {
            'statusCode': 200,
            'headers': CORS_HEADERS,
            'body': json.dumps({
                'trending_tags': trending_tags
            })
        }
    
    except Exception as e:
        print("Error executing query: " + str(e))
        return {
            'statusCode': 500,
            'headers': CORS_HEADERS,
            'body': json.dumps({
                'error': 'Query execution failed',
                'details': str(e),
                'status': False
            })
        }
    
    finally:
        connection.close()
        print("Database connection closed")


