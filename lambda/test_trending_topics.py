"""
Test script for get_trending_topics Lambda function.

This script tests the Lambda function locally by:
1. Mocking the database connection with test data
2. Testing various input scenarios
3. Validating the output format

Usage:
    python test_trending_topics.py

For testing with actual database:
    Set environment variables: DB_HOST, DB_USER, DB_PASSWORD, DB_NAME, DB_PORT
    Then run: python test_trending_topics.py --live
"""

import json
import sys
from unittest.mock import MagicMock, patch
from collections import Counter


def create_mock_cursor(mock_data):
    """Create a mock cursor that returns the specified data."""
    cursor = MagicMock()
    cursor.fetchall.return_value = mock_data
    cursor.__enter__ = MagicMock(return_value=cursor)
    cursor.__exit__ = MagicMock(return_value=False)
    return cursor


def create_mock_connection(mock_data):
    """Create a mock database connection."""
    connection = MagicMock()
    connection.cursor.return_value = create_mock_cursor(mock_data)
    return connection


def test_successful_trending_topics():
    """Test successful retrieval of trending topics."""
    print("\n=== Test: Successful Trending Topics ===")
    
    # Mock data: sessions with tags within radius
    mock_sessions = [
        {'tag': 'pain_relief'},
        {'tag': 'pain_relief'},
        {'tag': 'pain_relief'},
        {'tag': 'headache'},
        {'tag': 'headache'},
        {'tag': 'fever'},
        {'tag': 'cold'},
        {'tag': 'sore_throat'},
        {'tag': 'pain_relief'},
        {'tag': 'headache'},
    ]
    
    with patch('pymysql.connect') as mock_connect:
        mock_connect.return_value = create_mock_connection(mock_sessions)
        
        from get_trending_topics import get_trending_topics
        
        # GET request with query string parameters
        event = {
            'queryStringParameters': {
                'latitude': '12.9716',
                'longitude': '77.5946',
                'radius': '10'
            }
        }
        
        result = get_trending_topics(event, None)
        body = json.loads(result['body'])

        assert result['statusCode'] == 200, f"Expected 200, got {result['statusCode']}"
        assert 'trending_tags' in body, "Expected trending_tags in response"
        assert len(body['trending_tags']) <= 5, "Expected at most 5 trending tags"

        # Verify the tags are sorted by count
        tags = body['trending_tags']
        for i in range(len(tags) - 1):
            assert tags[i]['count'] >= tags[i+1]['count'], "Tags should be sorted by count descending"

        print("✓ Response:", json.dumps(body, indent=2))
        print("✓ Test passed!")


def test_missing_latitude():
    """Test error handling when latitude is missing."""
    print("\n=== Test: Missing Latitude ===")
    
    with patch('pymysql.connect') as mock_connect:
        mock_connect.return_value = create_mock_connection([])
        
        from get_trending_topics import get_trending_topics
        
        event = {
            'queryStringParameters': {
                'longitude': '77.5946',
                'radius': '10'
            }
        }
        
        result = get_trending_topics(event, None)
        
        assert result['statusCode'] == 400, f"Expected 400, got {result['statusCode']}"
        body = json.loads(result['body'])
        assert body['status'] == False, "Expected status to be False"
        
        print("✓ Response:", json.dumps(body, indent=2))
        print("✓ Test passed!")


def test_invalid_latitude():
    """Test error handling when latitude is out of range."""
    print("\n=== Test: Invalid Latitude (out of range) ===")
    
    with patch('pymysql.connect') as mock_connect:
        mock_connect.return_value = create_mock_connection([])
        
        from get_trending_topics import get_trending_topics
        
        event = {
            'queryStringParameters': {
                'latitude': '100',  # Invalid: > 90
                'longitude': '77.5946',
                'radius': '10'
            }
        }
        
        result = get_trending_topics(event, None)
        
        assert result['statusCode'] == 400, f"Expected 400, got {result['statusCode']}"
        body = json.loads(result['body'])
        assert body['status'] == False, "Expected status to be False"
        assert 'latitude' in body['error'].lower(), "Error should mention latitude"
        
        print("✓ Response:", json.dumps(body, indent=2))
        print("✓ Test passed!")


def test_invalid_longitude():
    """Test error handling when longitude is out of range."""
    print("\n=== Test: Invalid Longitude (out of range) ===")
    
    with patch('pymysql.connect') as mock_connect:
        mock_connect.return_value = create_mock_connection([])
        
        from get_trending_topics import get_trending_topics
        
        event = {
            'queryStringParameters': {
                'latitude': '12.9716',
                'longitude': '200',  # Invalid: > 180
                'radius': '10'
            }
        }
        
        result = get_trending_topics(event, None)
        
        assert result['statusCode'] == 400, f"Expected 400, got {result['statusCode']}"
        body = json.loads(result['body'])
        assert body['status'] == False, "Expected status to be False"
        assert 'longitude' in body['error'].lower(), "Error should mention longitude"
        
        print("✓ Response:", json.dumps(body, indent=2))
        print("✓ Test passed!")


def test_invalid_radius():
    """Test error handling when radius is invalid."""
    print("\n=== Test: Invalid Radius (zero or negative) ===")
    
    with patch('pymysql.connect') as mock_connect:
        mock_connect.return_value = create_mock_connection([])
        
        from get_trending_topics import get_trending_topics
        
        event = {
            'queryStringParameters': {
                'latitude': '12.9716',
                'longitude': '77.5946',
                'radius': '-5'  # Invalid: negative
            }
        }
        
        result = get_trending_topics(event, None)
        
        assert result['statusCode'] == 400, f"Expected 400, got {result['statusCode']}"
        body = json.loads(result['body'])
        assert body['status'] == False, "Expected status to be False"
        
        print("✓ Response:", json.dumps(body, indent=2))
        print("✓ Test passed!")


def test_default_radius():
    """Test that default radius is used when not provided."""
    print("\n=== Test: Default Radius ===")
    
    mock_sessions = [{'tag': 'test_tag'}]
    
    with patch('pymysql.connect') as mock_connect:
        mock_connect.return_value = create_mock_connection(mock_sessions)
        
        from get_trending_topics import get_trending_topics
        
        event = {
            'queryStringParameters': {
                'latitude': '12.9716',
                'longitude': '77.5946'
                # radius not provided - should default to 10
            }
        }
        
        result = get_trending_topics(event, None)
        
        assert result['statusCode'] == 200, f"Expected 200, got {result['statusCode']}"
        body = json.loads(result['body'])
        
        print("✓ Response:", json.dumps(body, indent=2))
        print("✓ Test passed!")


def test_empty_results():
    """Test handling of no sessions found within radius."""
    print("\n=== Test: Empty Results (no sessions in radius) ===")
    
    with patch('pymysql.connect') as mock_connect:
        mock_connect.return_value = create_mock_connection([])
        
        from get_trending_topics import get_trending_topics
        
        event = {
            'queryStringParameters': {
                'latitude': '12.9716',
                'longitude': '77.5946',
                'radius': '10'
            }
        }
        
        result = get_trending_topics(event, None)
        
        assert result['statusCode'] == 200, f"Expected 200, got {result['statusCode']}"
        body = json.loads(result['body'])
        assert body['trending_tags'] == [], "Expected empty trending_tags"
        
        print("✓ Response:", json.dumps(body, indent=2))
        print("✓ Test passed!")


def test_null_query_params():
    """Test error handling for null/missing query parameters."""
    print("\n=== Test: Null Query Parameters ===")
    
    with patch('pymysql.connect') as mock_connect:
        mock_connect.return_value = create_mock_connection([])
        
        from get_trending_topics import get_trending_topics
        
        event = {
            'queryStringParameters': None
        }
        
        result = get_trending_topics(event, None)
        
        assert result['statusCode'] == 400, f"Expected 400, got {result['statusCode']}"
        body = json.loads(result['body'])
        assert body['status'] == False, "Expected status to be False"
        
        print("✓ Response:", json.dumps(body, indent=2))
        print("✓ Test passed!")


def test_cors_headers():
    """Test that CORS headers are present in response."""
    print("\n=== Test: CORS Headers ===")
    
    with patch('pymysql.connect') as mock_connect:
        mock_connect.return_value = create_mock_connection([])
        
        from get_trending_topics import get_trending_topics
        
        event = {
            'queryStringParameters': {
                'latitude': '12.9716',
                'longitude': '77.5946',
                'radius': '10'
            }
        }
        
        result = get_trending_topics(event, None)
        
        assert 'headers' in result, "Expected headers in response"
        headers = result['headers']
        assert headers.get('Access-Control-Allow-Origin') == '*', "Missing CORS origin header"
        assert 'Access-Control-Allow-Methods' in headers, "Missing CORS methods header"
        assert 'GET' in headers.get('Access-Control-Allow-Methods', ''), "Should allow GET method"
        
        print("✓ CORS headers present")
        print("✓ Test passed!")


def run_live_test():
    """Run test against actual database (requires env variables)."""
    print("\n=== Live Database Test ===")
    print("Using database connection from environment variables...")
    
    import os
    required_vars = ['DB_HOST', 'DB_USER', 'DB_PASSWORD', 'DB_NAME']
    missing = [var for var in required_vars if not os.environ.get(var)]
    
    if missing:
        print(f"✗ Missing environment variables: {', '.join(missing)}")
        print("Set these variables and try again.")
        return False
    
    from get_trending_topics import get_trending_topics
    
    event = {
        'queryStringParameters': {
            'latitude': '12.9716',
            'longitude': '77.5946',
            'radius': '50'  # Larger radius for testing
        }
    }
    
    result = get_trending_topics(event, None)
    body = json.loads(result['body'])
    
    print(f"Status Code: {result['statusCode']}")
    print(f"Response: {json.dumps(body, indent=2)}")
    
    if result['statusCode'] == 200:
        print("✓ Live test passed!")
        return True
    else:
        print("✗ Live test failed!")
        return False


def main():
    """Run all tests."""
    print("=" * 60)
    print("Testing get_trending_topics Lambda Function")
    print("=" * 60)
    
    # Check if live test is requested
    if '--live' in sys.argv:
        run_live_test()
        return
    
    # Run mock tests
    tests = [
        test_successful_trending_topics,
        test_missing_latitude,
        test_invalid_latitude,
        test_invalid_longitude,
        test_invalid_radius,
        test_default_radius,
        test_empty_results,
        test_null_query_params,
        test_cors_headers,
    ]
    
    passed = 0
    failed = 0
    
    for test in tests:
        try:
            test()
            passed += 1
        except Exception as e:
            print(f"✗ Test failed with error: {e}")
            failed += 1
    
    print("\n" + "=" * 60)
    print(f"Results: {passed} passed, {failed} failed")
    print("=" * 60)
    
    if failed > 0:
        sys.exit(1)


if __name__ == "__main__":
    main()
