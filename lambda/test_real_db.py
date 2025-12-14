#!/usr/bin/env python3
"""
Simple script to test get_trending_topics Lambda function with real database.

Usage:
1. Set your database credentials in environment variables or create a .env file:
   DB_HOST=your_host
   DB_USER=your_user
   DB_PASSWORD=your_password
   DB_NAME=your_db
   DB_PORT=3306

2. Configure test parameters below (latitude, longitude, radius)

3. Run: python test_real_db.py
"""

import json
import os
import sys
from dotenv import load_dotenv

# Load environment variables from .env file if it exists
load_dotenv()

# === CONFIGURE YOUR TEST PARAMETERS HERE ===
TEST_LATITUDE = 40.84120000
TEST_LONGITUDE = -74.24670000
TEST_RADIUS = 50          # Search radius in miles
# === END CONFIGURATION ===

def load_env_vars():
    """Load and validate environment variables."""
    required_vars = ['DB_HOST', 'DB_USER', 'DB_PASSWORD', 'DB_NAME', 'DB_PORT']
    env_vars = {}

    missing = []
    for var in required_vars:
        value = os.environ.get(var)
        if not value:
            missing.append(var)
        else:
            env_vars[var] = value

    if missing:
        print("❌ Missing environment variables:")
        for var in missing:
            print(f"   {var}")
        print("\nPlease set these in your environment or .env file.")
        sys.exit(1)

    print("✅ Database credentials loaded")
    return env_vars

def test_trending_topics():
    """Test the trending topics function with real data."""

    # Load credentials
    env_vars = load_env_vars()

    # Set environment variables for the Lambda function
    os.environ.update(env_vars)

    print(f"\n🔍 Testing with coordinates: {TEST_LATITUDE}, {TEST_LONGITUDE}")
    print(f"📍 Search radius: {TEST_RADIUS} miles")
    print("=" * 60)

    try:
        from get_trending_topics import get_trending_topics

        # Prepare the event (simulating API Gateway GET request)
        event = {
            'queryStringParameters': {
                'latitude': str(TEST_LATITUDE),
                'longitude': str(TEST_LONGITUDE),
                'radius': str(TEST_RADIUS)
            }
        }

        print("🚀 Calling Lambda function...")

        # Call the function
        result = get_trending_topics(event, None)

        # Parse and display results
        print(f"\n📊 Status Code: {result['statusCode']}")

        if result['statusCode'] == 200:
            body = json.loads(result['body'])
            trending_tags = body.get('trending_tags', [])

            print("✅ Success!")
            print(f"🏷️  Found {len(trending_tags)} trending tags:")

            if trending_tags:
                print("\n📈 Trending Topics:")
                print("-" * 30)
                for i, tag_info in enumerate(trending_tags, 1):
                    print(f"{i:2d}. {tag_info["tag"]}      (count: {tag_info["count"]})")
            else:
                print("\n📭 No trending topics found in this area.")

        else:
            body = json.loads(result['body'])
            print("❌ HTTP Error:")
            print(f"   {body.get('error', 'Unknown error')}")

    except Exception as e:
        print(f"❌ Error calling Lambda function: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)

def main():
    """Main function."""
    print("🌟 Real Database Test for get_trending_topics Lambda")
    print("=" * 60)

    test_trending_topics()

    print("\n" + "=" * 60)
    print("🎉 Test completed!")

if __name__ == "__main__":
    main()