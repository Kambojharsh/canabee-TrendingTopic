import streamlit as st
from openai import OpenAI
from typing import List, Dict, Tuple, Optional
from PIL import ImageDraw, ImageFont
import os
from ddgs import DDGS
import requests
from io import BytesIO
from PIL import Image
import time
import hashlib
from functools import lru_cache
import base64
from urllib.parse import quote_plus
import re
import sqlite3
from datetime import datetime
import xml.etree.ElementTree as ET
import boto3
from botocore.exceptions import ClientError, NoCredentialsError
import json

# Configuration file path
CONFIG_FILE = "config.json"

def load_config() -> Dict:
    """
    Load configuration from config.json file with fallback to environment variables.
    Returns a dictionary with configuration values.
    """
    config = {
        "openai": {
            "api_key": "",
            "enabled": False
        },
        "pubmed": {
            "api_key": "",
            "enabled": False
        },
        "aws": {
            "access_key_id": "",
            "secret_access_key": "",
            "region": "us-east-1",
            "s3_bucket": "",
            "s3_prefix": "",
            "enabled": False
        },
        "features": {
            "web_search": {
                "enabled": True
            },
            "pubmed_search": {
                "enabled": True
            },
            "auto_upload_to_s3": {
                "enabled": False
            }
        }
    }
    
    # Try to load from config file
    if os.path.exists(CONFIG_FILE):
        try:
            with open(CONFIG_FILE, 'r') as f:
                file_config = json.load(f)
                # Merge file config with defaults
                if "openai" in file_config:
                    config["openai"].update(file_config["openai"])
                if "pubmed" in file_config:
                    config["pubmed"].update(file_config["pubmed"])
                if "aws" in file_config:
                    config["aws"].update(file_config["aws"])
                if "features" in file_config:
                    config["features"].update(file_config["features"])
        except Exception as e:
            st.warning(f"⚠️ Error loading config.json: {str(e)}. Using defaults and environment variables.")
    
    # Fallback to environment variables if config values are empty
    if not config["openai"]["api_key"]:
        config["openai"]["api_key"] = os.getenv("OPENAI_API_KEY", "")
    
    if not config["pubmed"]["api_key"]:
        config["pubmed"]["api_key"] = os.getenv("PUBMED_API_KEY", "")
    
    if not config["aws"]["access_key_id"]:
        config["aws"]["access_key_id"] = os.getenv("AWS_ACCESS_KEY_ID", "")
    
    if not config["aws"]["secret_access_key"]:
        config["aws"]["secret_access_key"] = os.getenv("AWS_SECRET_ACCESS_KEY", "")
    
    if not config["aws"]["region"]:
        config["aws"]["region"] = os.getenv("AWS_REGION", "us-east-1")
    
    if not config["aws"]["s3_bucket"]:
        config["aws"]["s3_bucket"] = os.getenv("S3_BUCKET", "")
    
    if not config["aws"].get("s3_prefix"):
        config["aws"]["s3_prefix"] = os.getenv("S3_PREFIX", "")
    
    # Clean up bucket name (remove s3:// prefix if present)
    if config["aws"]["s3_bucket"].startswith("s3://"):
        bucket_path = config["aws"]["s3_bucket"].replace("s3://", "").rstrip("/")
        parts = bucket_path.split("/", 1)
        config["aws"]["s3_bucket"] = parts[0]
        if len(parts) > 1 and not config["aws"]["s3_prefix"]:
            config["aws"]["s3_prefix"] = parts[1] + "/"
    
    # Determine if features should be enabled based on credentials
    config["openai"]["enabled"] = bool(config["openai"]["api_key"])
    config["pubmed"]["enabled"] = bool(config["pubmed"]["api_key"])
    config["aws"]["enabled"] = bool(config["aws"]["access_key_id"] and config["aws"]["secret_access_key"] and config["aws"]["s3_bucket"])
    
    # Auto-upload requires AWS to be enabled
    if not config["aws"]["enabled"]:
        config["features"]["auto_upload_to_s3"]["enabled"] = False
    
    return config

def save_config(config: Dict):
    """
    Save configuration to config.json file.
    """
    try:
        # Create a safe copy without exposing secrets in UI
        safe_config = {
            "openai": {
                "api_key": config["openai"]["api_key"],
                "enabled": config["openai"]["enabled"]
            },
            "pubmed": {
                "api_key": config["pubmed"]["api_key"],
                "enabled": config["pubmed"]["enabled"]
            },
            "aws": {
                "access_key_id": config["aws"]["access_key_id"],
                "secret_access_key": config["aws"]["secret_access_key"],
                "region": config["aws"]["region"],
                "s3_bucket": config["aws"]["s3_bucket"],
                "enabled": config["aws"]["enabled"]
            },
            "features": config["features"]
        }
        
        with open(CONFIG_FILE, 'w') as f:
            json.dump(safe_config, f, indent=2)
        return True
    except Exception as e:
        st.error(f"Error saving config.json: {str(e)}")
        return False

# Load configuration
app_config = load_config()

# Page configuration
st.set_page_config(
    page_title="Cannabis Chat Assistant POC",
    page_icon="🌿",
    layout="wide"
)

# Initialize session state
if "messages" not in st.session_state:
    st.session_state.messages = []

if "api_key" not in st.session_state:
    st.session_state.api_key = app_config["openai"]["api_key"]

if "model" not in st.session_state:
    st.session_state.model = "gpt-5.1"  # Default to GPT-5.1

if "config" not in st.session_state:
    st.session_state.config = app_config

if "search_results" not in st.session_state:
    st.session_state.search_results = {}  # Store search results with images per message

if "search_cache" not in st.session_state:
    st.session_state.search_cache = {}  # Cache search results to reduce API calls

if "last_search_time" not in st.session_state:
    st.session_state.last_search_time = 0  # Track last search time for rate limiting

# Cannabis-related keywords for guardrails
CANNABIS_KEYWORDS = [
    "cannabis", "marijuana", "weed", "hemp", "cbd", "thc", "cannabinoid",
    "edible", "tincture", "vape", "concentrate", "flower", "bud", "strain",
    "indica", "sativa", "hybrid", "terpene", "terpenes", "dispensary",
    "cultivation", "growing", "harvest", "extraction", "hash", "kief",
    "pre-roll", "preroll", "joint", "blunt", "bong", "pipe", "dab",
    "medical marijuana", "recreational", "legalization", "cannabidiol"
]

# Reputable cannabis sources (domains to prioritize)
REPUTABLE_SOURCES = [
    "leafly.com", "weedmaps.com", "hightimes.com", "cannabisnow.com",
    "medicalnewstoday.com", "healthline.com", "webmd.com", "mayoclinic.org",
    "ncbi.nlm.nih.gov", "pubmed.ncbi.nlm.nih.gov", "fda.gov", "cdc.gov",
    "norml.org", "mpp.org", "cannabisindustryjournal.com", "mjbusiness.com",
    "cannabistech.com", "greenmarketreport.com", "cannabisbusinessexecutive.com",
    "projectcbd.org", "hempindustrydaily.com", "cannabisdispensary.com"
]

def check_cannabis_relevance(question: str) -> Tuple[bool, str]:
    """
    Cannabis guardrails: Check if question is related to cannabis.
    Returns (is_relevant, message)
    """
    question_lower = question.lower()
    
    # Check for cannabis-related keywords
    has_keyword = any(keyword in question_lower for keyword in CANNABIS_KEYWORDS)
    
    if not has_keyword:
        return False, "⚠️ This chat assistant is restricted to cannabis-related questions only. Please ask about cannabis, CBD, THC, strains, products, cultivation, or related topics."
    
    return True, ""

def is_reputable_source(url: str) -> bool:
    """
    Check if a URL is from a reputable source.
    """
    if not url:
        return False
    url_lower = url.lower()
    return any(reputable_domain in url_lower for reputable_domain in REPUTABLE_SOURCES)

def prioritize_reputable_sources(results: List[Dict]) -> List[Dict]:
    """
    Sort results to prioritize reputable sources.
    """
    reputable = []
    others = []
    
    for result in results:
        url = result.get('href', '')
        if is_reputable_source(url):
            reputable.append(result)
        else:
            others.append(result)
    
    # Return reputable sources first, then others
    return reputable + others

def get_cache_key(query: str, search_type: str = "text") -> str:
    """
    Generate a cache key for a search query.
    """
    key_string = f"{search_type}:{query.lower().strip()}"
    return hashlib.md5(key_string.encode()).hexdigest()

def perform_web_search_with_retry(ddgs, query: str, max_results: int, search_type: str = "text", max_retries: int = 3) -> List[Dict]:
    """
    Perform a web search with retry logic and rate limiting.
    """
    for attempt in range(max_retries):
        try:
            # Rate limiting: wait between requests
            current_time = time.time()
            time_since_last_search = current_time - st.session_state.last_search_time
            min_delay = 1.0  # Minimum 1 second between searches
            
            if time_since_last_search < min_delay:
                time.sleep(min_delay - time_since_last_search)
            
            # Perform the search
            if search_type == "text":
                search_results = ddgs.text(query, max_results=max_results)
                results = list(search_results) if search_results else []
            elif search_type == "images":
                search_results = ddgs.images(query, max_results=max_results)
                results = list(search_results) if search_results else []
            else:
                results = []
            
            # Update last search time
            st.session_state.last_search_time = time.time()
            
            # Debug: log if no results
            if not results and attempt == max_retries - 1:
                st.info(f"ℹ️ No results found for query: '{query}'")
            
            return results
            
        except Exception as e:
            error_str = str(e).lower()
            
            # Check for rate limiting indicators
            if "rate limit" in error_str or "429" in error_str or "too many" in error_str:
                if attempt < max_retries - 1:
                    # Exponential backoff: wait longer on each retry
                    wait_time = (2 ** attempt) + 1  # 2s, 3s, 5s
                    st.warning(f"⚠️ Rate limit detected. Waiting {wait_time} seconds before retry {attempt + 1}/{max_retries}...")
                    time.sleep(wait_time)
                    continue
                else:
                    st.error("❌ Rate limit exceeded. Please wait a moment before searching again.")
                    return []
            else:
                # Other errors - log and return empty
                if attempt == max_retries - 1:
                    error_details = f"Search error (attempt {attempt + 1}/{max_retries}): {str(e)}"
                    st.warning(f"⚠️ {error_details}")
                    # Log full error for debugging
                    import traceback
                    st.info(f"Full error traceback: {traceback.format_exc()}")
                return []
    
    return []

def filter_vector_images(image_results: List[Dict]) -> List[Dict]:
    """
    Filter out vector graphics, infographics, and illustration images.
    Keep only actual product photos.
    """
    if not image_results:
        return []
    
    # Domains to exclude (vector/infographic sites)
    vector_domains = [
        'vecteezy.com',
        'dreamstime.com',
        'shutterstock.com',
        'freepik.com',
        '123rf.com',
        'istockphoto.com',
        'gettyimages.com',
        'adobe.com',
        'depositphotos.com',
        'alamy.com'
    ]
    
    # Keywords that indicate vector/infographic content
    vector_keywords = [
        'vector',
        'infographic',
        'illustration',
        'clip-art',
        'clip art',
        'scheme',
        'diagram',
        'icon',
        'graphic design',
        'flat design',
        'cartoon',
        'drawing',
        'sketch'
    ]
    
    filtered_results = []
    for img_result in image_results:
        img_url = img_result.get('image', '').lower()
        source_url = img_result.get('url', img_result.get('href', '')).lower()
        title = img_result.get('title', '').lower()
        
        # Check if from vector domain
        is_vector_domain = any(domain in img_url or domain in source_url for domain in vector_domains)
        
        # Check if contains vector keywords
        all_text = f"{img_url} {source_url} {title}"
        has_vector_keywords = any(keyword in all_text for keyword in vector_keywords)
        
        # Only include if NOT a vector/infographic
        if not is_vector_domain and not has_vector_keywords:
            filtered_results.append(img_result)
    
    return filtered_results


def perform_web_search(query: str, max_results: int = 8) -> Tuple[List[Dict], List[Dict]]:
    """
    Perform web search about cannabis using DuckDuckGo with rate limiting and caching.
    Returns (text_results, image_results) with reputable sources prioritized.
    """
    text_results = []
    image_results = []
    
    # Check cache first
    text_cache_key = get_cache_key(query, "text")
    if text_cache_key in st.session_state.search_cache:
        cached_result = st.session_state.search_cache[text_cache_key]
        if cached_result.get("timestamp", 0) > time.time() - 300:  # Cache for 5 minutes
            text_results = cached_result.get("results", [])
            st.info("📦 Using cached search results")
    
    try:
        # Only perform search if not cached
        if not text_results:
            with DDGS() as ddgs:
                search_query = f"{query} cannabis"
                
                # Text search with retry logic
                text_results = perform_web_search_with_retry(
                    ddgs, search_query, max_results * 2, "text"
                )
                
                # Cache the results
                if text_results:
                    st.session_state.search_cache[text_cache_key] = {
                        "results": text_results,
                        "timestamp": time.time()
                    }
                
                # Prioritize reputable sources
                text_results = prioritize_reputable_sources(text_results)
                # Take top results after prioritization
                text_results = text_results[:max_results]
        
        # Image search for product recommendations (with rate limiting)
        # Always search for products unless query explicitly excludes them
        exclude_keywords = ["research only", "study only", "no products", "books only"]
        should_exclude_products = any(keyword in query.lower() for keyword in exclude_keywords)
        
        # Search for products for most cannabis queries
        if not should_exclude_products:
            # Check image cache
            image_cache_key = get_cache_key(query, "images")
            if image_cache_key in st.session_state.search_cache:
                cached_result = st.session_state.search_cache[image_cache_key]
                if cached_result.get("timestamp", 0) > time.time() - 300:  # Cache for 5 minutes
                    cached_images = cached_result.get("results", [])
                    # Filter cached results too (in case filter was added after caching)
                    image_results = filter_vector_images(cached_images)
            
            # Only search if not cached or filtered results are empty
            if not image_results:
                with DDGS() as ddgs:
                    # Try multiple query variations to get product images
                    # First try: direct product query
                    image_query = f"{query} cannabis product photo"
                    image_results = perform_web_search_with_retry(
                        ddgs, image_query, 12, "images"  # Get more results to filter from
                    )
                    
                    # If no results, try alternative queries
                    if not image_results:
                        # Try with "buy" or "shop" keywords
                        alt_queries = [
                            f"{query} cannabis buy",
                            f"{query} CBD product",
                            f"{query} cannabis shop"
                        ]
                        for alt_query in alt_queries:
                            temp_results = perform_web_search_with_retry(
                                ddgs, alt_query, 8, "images"
                            )
                            if temp_results:
                                image_results = temp_results
                                break
                    
                    # Filter out vector/infographic images
                    image_results = filter_vector_images(image_results)
                    
                    # Limit to top 4 after filtering
                    image_results = image_results[:4]
                    
                    # Cache the filtered results
                    if image_results:
                        st.session_state.search_cache[image_cache_key] = {
                            "results": image_results,
                            "timestamp": time.time()
                        }
            
    except Exception as e:
        error_msg = str(e).lower()
        if "rate limit" in error_msg or "429" in error_msg:
            st.error("❌ DuckDuckGo rate limit exceeded. Please wait a few moments before searching again.")
        else:
            st.error(f"Search error: {str(e)}")
            # Provide more context
            if not text_results and not image_results:
                st.info("💡 Tip: Try clearing the search cache or waiting a moment before searching again.")
    
    # Debug: Show if we have results
    if text_results:
        st.success(f"✅ Found {len(text_results)} text results")
    if image_results:
        st.success(f"✅ Found {len(image_results)} image results")
    
    return text_results, image_results

def extract_keywords_for_pubmed(user_question: str) -> str:
    """
    Extract key terms from user question for PubMed search.
    Focuses on medical conditions, symptoms, and cannabis-related terms.
    """
    # Common medical condition keywords
    medical_keywords = [
        "pain", "chronic", "acute", "inflammation", "arthritis", "anxiety", 
        "depression", "sleep", "insomnia", "nausea", "migraine", "headache",
        "knee", "shoulder", "back", "neck", "joint", "muscle", "spasm",
        "epilepsy", "seizure", "cancer", "tumor", "glaucoma", "diabetes",
        "ptsd", "ptsd", "autism", "adhd", "fibromyalgia", "ms", "multiple sclerosis"
    ]
    
    # Extract relevant keywords from question
    question_lower = user_question.lower()
    found_keywords = []
    
    # Look for medical conditions
    for keyword in medical_keywords:
        if keyword in question_lower:
            found_keywords.append(keyword)
    
    # Always include cannabis-related terms
    cannabis_terms = ["cannabis", "cannabinoid", "cbd", "thc", "marijuana", "hemp"]
    
    # Build search query
    if found_keywords:
        # Use found keywords + cannabis
        query_parts = found_keywords[:3]  # Limit to top 3 keywords
        query_parts.append("cannabis")
        pubmed_query = " ".join(query_parts)
    else:
        # Fallback: use simplified question + cannabis
        # Remove common words and keep meaningful terms
        words = question_lower.split()
        stop_words = {"i", "am", "need", "for", "a", "an", "the", "to", "is", "are", "was", "were", "be", "been", "being", "have", "has", "had", "do", "does", "did", "will", "would", "should", "could", "can", "may", "might", "must", "shall"}
        meaningful_words = [w for w in words if w not in stop_words and len(w) > 2][:4]
        if meaningful_words:
            meaningful_words.append("cannabis")
            pubmed_query = " ".join(meaningful_words)
        else:
            pubmed_query = "cannabis"
    
    return pubmed_query

# NCBI E-utilities base URL
NCBI_EUTILS_BASE = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/"

# SQLite database file path
DB_FILE = "pubmed_articles.db"

def init_pubmed_database():
    """
    Initialize SQLite database for storing PubMed article metadata.
    """
    conn = sqlite3.connect(DB_FILE)
    cursor = conn.cursor()
    
    cursor.execute("""
        CREATE TABLE IF NOT EXISTS pubmed_articles (
            pmid TEXT PRIMARY KEY,
            authors TEXT,
            journal TEXT,
            published_date TEXT,
            doi TEXT,
            pdf_path TEXT,
            s3_key TEXT,
            s3_bucket TEXT,
            uploaded_to_s3 INTEGER DEFAULT 0,
            s3_upload_date TIMESTAMP,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    """)
    
    # Add S3 columns if they don't exist (for existing databases)
    try:
        cursor.execute("ALTER TABLE pubmed_articles ADD COLUMN s3_key TEXT")
    except sqlite3.OperationalError:
        pass  # Column already exists
    
    try:
        cursor.execute("ALTER TABLE pubmed_articles ADD COLUMN s3_bucket TEXT")
    except sqlite3.OperationalError:
        pass  # Column already exists
    
    try:
        cursor.execute("ALTER TABLE pubmed_articles ADD COLUMN uploaded_to_s3 INTEGER DEFAULT 0")
    except sqlite3.OperationalError:
        pass  # Column already exists
    
    try:
        cursor.execute("ALTER TABLE pubmed_articles ADD COLUMN s3_upload_date TIMESTAMP")
    except sqlite3.OperationalError:
        pass  # Column already exists
    
    conn.commit()
    conn.close()

def pmid_exists(pmid: str) -> bool:
    """
    Check if a PMID already exists in the database.
    """
    conn = sqlite3.connect(DB_FILE)
    cursor = conn.cursor()
    
    cursor.execute("SELECT 1 FROM pubmed_articles WHERE pmid = ?", (pmid,))
    exists = cursor.fetchone() is not None
    
    conn.close()
    return exists

def store_pmid_data(pmid: str, authors: str, journal: str, published_date: str, doi: Optional[str] = None, pdf_path: Optional[str] = None, s3_key: Optional[str] = None, s3_bucket: Optional[str] = None, uploaded_to_s3: bool = False):
    """
    Store PMID data in the database.
    """
    conn = sqlite3.connect(DB_FILE)
    cursor = conn.cursor()
    
    cursor.execute("""
        INSERT OR REPLACE INTO pubmed_articles 
        (pmid, authors, journal, published_date, doi, pdf_path, s3_key, s3_bucket, uploaded_to_s3, s3_upload_date, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
    """, (pmid, authors, journal, published_date, doi or "", pdf_path or "", s3_key or "", s3_bucket or "", 1 if uploaded_to_s3 else 0, datetime.now().isoformat() if uploaded_to_s3 else None))
    
    conn.commit()
    conn.close()

def get_s3_client(aws_access_key_id: Optional[str] = None, aws_secret_access_key: Optional[str] = None, region_name: str = "us-east-1"):
    """
    Create and return an S3 client.
    """
    try:
        if aws_access_key_id and aws_secret_access_key:
            s3_client = boto3.client(
                's3',
                aws_access_key_id=aws_access_key_id,
                aws_secret_access_key=aws_secret_access_key,
                region_name=region_name
            )
        else:
            # Use default credentials from environment or IAM role
            s3_client = boto3.client('s3', region_name=region_name)
        return s3_client
    except Exception as e:
        st.error(f"Error creating S3 client: {str(e)}")
        return None

def upload_file_to_s3(file_path: str, bucket_name: str, s3_key: Optional[str] = None, aws_access_key_id: Optional[str] = None, aws_secret_access_key: Optional[str] = None, region_name: str = "us-east-1", s3_prefix: Optional[str] = None) -> Tuple[bool, Optional[str], Optional[str]]:
    """
    Upload a file to S3 bucket.
    Returns (success, s3_key, error_message)
    """
    if not os.path.exists(file_path):
        return False, None, f"File not found: {file_path}"
    
    if not bucket_name:
        return False, None, "S3 bucket name not configured"
    
    try:
        s3_client = get_s3_client(aws_access_key_id, aws_secret_access_key, region_name)
        if not s3_client:
            return False, None, "Failed to create S3 client"
        
        # Generate S3 key if not provided
        if not s3_key:
            filename = os.path.basename(file_path)
            # Use prefix from parameter, or try to get from config
            if s3_prefix:
                prefix = s3_prefix.rstrip("/") + "/"
            else:
                # Try to get from session state config if available
                try:
                    config = st.session_state.get("config", {})
                    aws_config = config.get("aws", {})
                    prefix_val = aws_config.get("s3_prefix", "")
                    prefix = prefix_val.rstrip("/") + "/" if prefix_val else ""
                except:
                    prefix = ""
            s3_key = f"{prefix}pubmed_articles/{filename}"
        elif s3_prefix and not s3_key.startswith(s3_prefix):
            # If s3_key is provided but doesn't include prefix, prepend it
            prefix = s3_prefix.rstrip("/") + "/"
            s3_key = f"{prefix}{s3_key}"
        
        # Upload file
        s3_client.upload_file(file_path, bucket_name, s3_key)
        
        st.info(f"🔍 DEBUG: Upload successful!")
        return True, s3_key, None
        
    except NoCredentialsError as e:
        error_msg = f"AWS credentials not found. Please configure AWS access key and secret key. Error: {str(e)}"
        st.error(f"❌ {error_msg}")
        st.info(f"🔍 DEBUG: NoCredentialsError - {str(e)}")
        return False, None, error_msg
    except ClientError as e:
        error_code = e.response.get('Error', {}).get('Code', 'Unknown')
        error_message = e.response.get('Error', {}).get('Message', str(e))
        error_msg = f"AWS S3 error ({error_code}): {error_message}"
        st.error(f"❌ {error_msg}")
        st.info(f"🔍 DEBUG: ClientError - Code: {error_code}, Message: {error_message}")
        st.info(f"🔍 DEBUG: Full error response: {e.response}")
        return False, None, error_msg
    except Exception as e:
        error_msg = f"Error uploading to S3: {str(e)}"
        st.error(f"❌ {error_msg}")
        st.info(f"🔍 DEBUG: Exception - {type(e).__name__}: {str(e)}")
        import traceback
        st.info(f"🔍 DEBUG: Traceback: {traceback.format_exc()}")
        return False, None, error_msg

def check_s3_upload_status(pmid: str) -> Tuple[bool, Optional[str], Optional[str]]:
    """
    Check if a PMID has been uploaded to S3.
    Returns (is_uploaded, s3_key, s3_bucket)
    """
    conn = sqlite3.connect(DB_FILE)
    cursor = conn.cursor()
    
    cursor.execute("""
        SELECT uploaded_to_s3, s3_key, s3_bucket 
        FROM pubmed_articles 
        WHERE pmid = ?
    """, (pmid,))
    
    result = cursor.fetchone()
    conn.close()
    
    if result:
        uploaded_to_s3 = bool(result[0])
        s3_key = result[1] if result[1] else None
        s3_bucket = result[2] if result[2] else None
        return uploaded_to_s3, s3_key, s3_bucket
    
    return False, None, None

def update_s3_upload_status(pmid: str, s3_key: str, s3_bucket: str):
    """
    Update S3 upload status in the database.
    """
    conn = sqlite3.connect(DB_FILE)
    cursor = conn.cursor()
    
    cursor.execute("""
        UPDATE pubmed_articles 
        SET s3_key = ?, s3_bucket = ?, uploaded_to_s3 = 1, s3_upload_date = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
        WHERE pmid = ?
    """, (s3_key, s3_bucket, pmid))
    
    conn.commit()
    conn.close()

def extract_title_for_filename(pubmed_root) -> str:
    """
    Extract and sanitize title from PubMed XML for use in filename.
    """
    title_elem = pubmed_root.find(".//ArticleTitle")
    if title_elem is not None and title_elem.text:
        title = title_elem.text
        safe_title = "".join(c for c in title[:50] if c.isalnum() or c in (' ', '-', '_')).strip()
        safe_title = safe_title.replace(' ', '_')
        return safe_title
    return "Article"

def extract_full_text_from_pmc_xml(pmc_root, pubmed_root, pmid: str) -> Optional[str]:
    """
    Extract full text from PMC XML and combine with PubMed metadata.
    Returns formatted full text document.
    """
    try:
        # Extract metadata from PubMed XML
        pubmed_article = pubmed_root.find(".//PubmedArticle")
        if pubmed_article is None:
            return None
        
        # Extract title
        title_elem = pubmed_article.find(".//ArticleTitle")
        title = title_elem.text if title_elem is not None else f"Article_{pmid}"
        
        # Extract authors
        authors = []
        for author in pubmed_article.findall(".//Author"):
            last_name_elem = author.find("LastName")
            first_name_elem = author.find("ForeName")
            if last_name_elem is not None and last_name_elem.text:
                last_name = last_name_elem.text
                first_name = first_name_elem.text if first_name_elem is not None and first_name_elem.text else ""
                if first_name:
                    authors.append(f"{first_name} {last_name}")
                else:
                    authors.append(last_name)
        
        # Extract journal
        journal_elem = pubmed_article.find(".//Journal/Title")
        journal = journal_elem.text if journal_elem is not None else "Unknown"
        
        # Extract publication date
        pub_date_elem = pubmed_article.find(".//PubDate")
        pub_date = "Unknown"
        if pub_date_elem is not None:
            year_elem = pub_date_elem.find("Year")
            month_elem = pub_date_elem.find("Month")
            day_elem = pub_date_elem.find("Day")
            if year_elem is not None and year_elem.text:
                date_parts = []
                if month_elem is not None and month_elem.text:
                    date_parts.append(month_elem.text)
                if day_elem is not None and day_elem.text:
                    date_parts.append(day_elem.text)
                date_parts.append(year_elem.text)
                pub_date = " ".join(date_parts)
        
        # Extract DOI
        doi_elem = pubmed_article.find(".//ELocationID[@EIdType='doi']")
        doi = doi_elem.text if doi_elem is not None else None
        
        # Extract abstract from PubMed
        abstract_parts = []
        abstract_elems = pubmed_article.findall(".//AbstractText")
        if abstract_elems:
            for abs_elem in abstract_elems:
                label = abs_elem.get("Label", "")
                text = "".join(abs_elem.itertext()).strip()
                if label:
                    abstract_parts.append(f"{label}: {text}")
                else:
                    abstract_parts.append(text)
        
        # Extract full text from PMC XML
        full_text_content = []
        
        # Find article body - try multiple possible locations
        body_elem = pmc_root.find(".//body")
        if body_elem is None:
            # Try alternative namespace or location
            body_elem = pmc_root.find(".//{http://www.ncbi.nlm.nih.gov/JATS1}body")
        if body_elem is None:
            # Try without namespace
            for elem in pmc_root.iter():
                if elem.tag.endswith('body') or elem.tag == 'body':
                    body_elem = elem
                    break
        
        if body_elem is not None:
            st.info(f"🔍 DEBUG: Found body element in PMC XML")
            # Extract all sections recursively
            sections = body_elem.findall(".//sec")
            # Also try with namespace
            if not sections:
                sections = body_elem.findall(".//{http://www.ncbi.nlm.nih.gov/JATS1}sec")
            # Also try direct sec children
            if not sections:
                sections = [s for s in body_elem if s.tag.endswith('sec') or s.tag == 'sec']
            
            st.info(f"🔍 DEBUG: Found {len(sections)} sections in PMC body")
            
            for section in sections:
                title_elem = section.find(".//title")
                if title_elem is None:
                    title_elem = section.find(".//{http://www.ncbi.nlm.nih.gov/JATS1}title")
                section_title = title_elem.text if title_elem is not None else ""
                
                # Extract all paragraphs in this section
                paragraphs = []
                for para in section.findall(".//p"):
                    para_text = "".join(para.itertext()).strip()
                    if para_text:
                        paragraphs.append(para_text)
                
                # Also try with namespace
                if not paragraphs:
                    for para in section.findall(".//{http://www.ncbi.nlm.nih.gov/JATS1}p"):
                        para_text = "".join(para.itertext()).strip()
                        if para_text:
                            paragraphs.append(para_text)
                
                # Extract lists
                for list_elem in section.findall(".//list"):
                    list_items = []
                    for item in list_elem.findall(".//list-item"):
                        item_text = "".join(item.itertext()).strip()
                        if item_text:
                            list_items.append(f"  • {item_text}")
                    if list_items:
                        paragraphs.extend(list_items)
                
                # Extract figure captions
                for fig_elem in section.findall(".//fig"):
                    caption_elem = fig_elem.find(".//caption")
                    if caption_elem is not None:
                        caption_text = "".join(caption_elem.itertext()).strip()
                        if caption_text:
                            paragraphs.append(f"[Figure: {caption_text}]")
                
                # Extract table captions and content
                for table_elem in section.findall(".//table-wrap"):
                    caption_elem = table_elem.find(".//caption")
                    if caption_elem is not None:
                        caption_text = "".join(caption_elem.itertext()).strip()
                        if caption_text:
                            paragraphs.append(f"[Table: {caption_text}]")
                    
                    # Extract table data
                    for table in table_elem.findall(".//table"):
                        for row in table.findall(".//tr"):
                            row_data = []
                            for cell in row.findall(".//td") + row.findall(".//th"):
                                cell_text = "".join(cell.itertext()).strip()
                                if cell_text:
                                    row_data.append(cell_text)
                            if row_data:
                                paragraphs.append(" | ".join(row_data))
                
                if section_title:
                    full_text_content.append(f"\n{section_title.upper()}\n")
                    full_text_content.append("=" * len(section_title) + "\n")
                
                for para in paragraphs:
                    full_text_content.append(para + "\n\n")
            
            # If no sections found, try to extract all text from body directly
            if not full_text_content:
                st.info(f"🔍 DEBUG: No sections found, extracting all text from body directly")
                body_text = "".join(body_elem.itertext()).strip()
                if body_text:
                    full_text_content.append(body_text)
                    st.info(f"🔍 DEBUG: Extracted {len(body_text)} characters from body")
        else:
            st.warning(f"⚠️ No body element found in PMC XML, trying alternative extraction")
            # Try to extract from article element directly
            article_elem = pmc_root.find(".//article")
            if article_elem is not None:
                article_text = "".join(article_elem.itertext()).strip()
                if article_text:
                    full_text_content.append(article_text)
                    st.info(f"🔍 DEBUG: Extracted {len(article_text)} characters from article element")
        
        # Build complete document
        doc_content = f"Title: {title}\n\n"
        doc_content += f"PMID: {pmid}\n"
        if doi:
            doc_content += f"DOI: {doi}\n"
        doc_content += f"Journal: {journal}\n"
        doc_content += f"Published: {pub_date}\n"
        doc_content += f"Authors: {', '.join(authors[:10])}{' et al.' if len(authors) > 10 else ''}\n\n"
        
        doc_content += "=" * 80 + "\n"
        doc_content += "ABSTRACT\n"
        doc_content += "=" * 80 + "\n\n"
        doc_content += "\n".join(abstract_parts) if abstract_parts else "No abstract available."
        doc_content += "\n\n"
        
        if full_text_content:
            doc_content += "=" * 80 + "\n"
            doc_content += "FULL TEXT\n"
            doc_content += "=" * 80 + "\n\n"
            doc_content += "".join(full_text_content)
        
        return doc_content
        
    except Exception as e:
        st.warning(f"⚠️ Error extracting full text from PMC XML: {str(e)}")
        return None

def fetch_pubmed_pdf(pmid: str, api_key: Optional[str] = None, upload_to_s3: bool = False, s3_bucket: Optional[str] = None, aws_access_key_id: Optional[str] = None, aws_secret_access_key: Optional[str] = None, aws_region: str = "us-east-1", s3_prefix: Optional[str] = None) -> Optional[str]:
    """
    Fetch PDF file or full text for a given PMID using eFetch.
    Optionally uploads to S3 after download.
    Returns the local file path if successful (PDF or TXT), None otherwise.
    """
    try:
        # Debug: Log upload parameters
        st.info(f"🔍 DEBUG: fetch_pubmed_pdf called for PMID {pmid}")
        st.info(f"🔍 DEBUG: upload_to_s3={upload_to_s3}, s3_bucket={s3_bucket}, s3_prefix={s3_prefix}")
        st.info(f"🔍 DEBUG: aws_access_key_id={'Set' if aws_access_key_id else 'Not set'}, aws_secret_access_key={'Set' if aws_secret_access_key else 'Not set'}")
        
        # Check if already uploaded to S3
        if upload_to_s3:
            is_uploaded, s3_key, s3_bucket_existing = check_s3_upload_status(pmid)
            if is_uploaded and s3_key and s3_bucket_existing:
                st.info(f"✅ PMID {pmid} already uploaded to S3: s3://{s3_bucket_existing}/{s3_key}")
                st.info(f"🔍 DEBUG: Skipping upload - already uploaded")
        
        # Check if PMID exists in database
        if pmid_exists(pmid):
            conn = sqlite3.connect(DB_FILE)
            cursor = conn.cursor()
            cursor.execute("SELECT pdf_path FROM pubmed_articles WHERE pmid = ?", (pmid,))
            result = cursor.fetchone()
            conn.close()
            
            if result and result[0] and os.path.exists(result[0]):
                file_path = result[0]
                # Upload to S3 if requested and not already uploaded
                if upload_to_s3 and s3_bucket:
                    st.info(f"🔍 DEBUG: Attempting to upload existing file: {file_path}")
                    is_uploaded, _, _ = check_s3_upload_status(pmid)
                    if not is_uploaded:
                        # Get prefix from parameter or config
                        if not s3_prefix:
                            try:
                                config = st.session_state.get("config", {})
                                s3_prefix = config.get("aws", {}).get("s3_prefix", "")
                                st.info(f"🔍 DEBUG: Got prefix from config: {s3_prefix}")
                            except Exception as e:
                                st.info(f"🔍 DEBUG: Error getting prefix from config: {e}")
                                s3_prefix = ""
                        st.info(f"🔍 DEBUG: Uploading with prefix: '{s3_prefix}'")
                        success, uploaded_s3_key, error = upload_file_to_s3(
                            file_path, s3_bucket, None, aws_access_key_id, aws_secret_access_key, aws_region, s3_prefix
                        )
                        if success:
                            update_s3_upload_status(pmid, uploaded_s3_key, s3_bucket)
                            st.success(f"✅ ✅ Successfully uploaded existing file to S3!")
                            st.success(f"✅ S3 Location: s3://{s3_bucket}/{uploaded_s3_key}")
                            st.info(f"📄 File: {file_path}")
                        else:
                            st.error(f"❌ S3 upload failed: {error}")
                            st.info(f"🔍 DEBUG: Upload error details - file: {file_path}, bucket: {s3_bucket}, prefix: {s3_prefix}")
                            st.info(f"🔍 DEBUG: File exists: {os.path.exists(file_path)}, File size: {os.path.getsize(file_path) if os.path.exists(file_path) else 0} bytes")
                    else:
                        st.info(f"🔍 DEBUG: File already uploaded, skipping")
                else:
                    if not upload_to_s3:
                        st.info(f"🔍 DEBUG: Upload disabled (upload_to_s3=False)")
                    if not s3_bucket:
                        st.info(f"🔍 DEBUG: No S3 bucket configured")
                return file_path  # Return existing file path
        
        # Create docs directory if it doesn't exist
        docs_dir = "docs"
        if not os.path.exists(docs_dir):
            os.makedirs(docs_dir, exist_ok=True)
        
        # Use eFetch to get article data - first get full PubMed record
        fetch_url = NCBI_EUTILS_BASE + "efetch.fcgi"
        fetch_params = {
            "db": "pubmed",
            "id": pmid,
            "retmode": "xml",
            "rettype": "full",  # Get full record, not just abstract
            "tool": "cannabis_chat_assistant",
            "email": "noreply@example.com"
        }
        
        if api_key:
            fetch_params["api_key"] = api_key.strip()
        
        # Rate limiting
        delay = 0.1 if api_key else 0.35
        time.sleep(delay)
        
        fetch_response = requests.get(fetch_url, params=fetch_params, timeout=30)
        
        if fetch_response.status_code != 200:
            st.warning(f"⚠️ Failed to fetch metadata for PMID {pmid}: {fetch_response.status_code}")
            return None
        
        # Parse XML to get article data
        try:
            root = ET.fromstring(fetch_response.content)
            
            # Try to get PMC ID for full text download
            # Check multiple possible locations for PMC ID
            pmc_id_elem = root.find(".//ArticleId[@IdType='pmc']")
            if pmc_id_elem is None:
                # Try alternative location
                pmc_id_elem = root.find(".//ArticleIdList/ArticleId[@IdType='pmc']")
            pmc_id = pmc_id_elem.text if pmc_id_elem is not None else None
            
            # Also check for PMCID in other formats
            if not pmc_id:
                for article_id in root.findall(".//ArticleId"):
                    id_type = article_id.get("IdType", "")
                    id_text = article_id.text if article_id.text else ""
                    if id_type.lower() == "pmc" or (id_text and id_text.upper().startswith("PMC")):
                        pmc_id = id_text
                        break
            
            st.info(f"🔍 DEBUG: PMC ID detection - Found: {pmc_id if pmc_id else 'None'}")
            
            # If PMC ID exists, try to download PDF first, then full text XML
            if pmc_id:
                # Remove "PMC" prefix if present
                pmc_id_clean = pmc_id.replace("PMC", "").strip()
                st.info(f"🔍 DEBUG: Attempting to fetch FULL TEXT from PMC ID: {pmc_id_clean}")
                
                # Try PDF download first
                pdf_url = f"https://www.ncbi.nlm.nih.gov/pmc/articles/PMC{pmc_id_clean}/pdf/"
                st.info(f"🔍 DEBUG: Trying PDF download from: {pdf_url}")
                pdf_response = requests.get(pdf_url, timeout=30, allow_redirects=True)
                
                st.info(f"🔍 DEBUG: PDF response status: {pdf_response.status_code}, content-type: {pdf_response.headers.get('content-type', 'unknown')}")
                
                if pdf_response.status_code == 200 and pdf_response.headers.get('content-type', '').startswith('application/pdf'):
                    pdf_path = os.path.join(docs_dir, f"PMID_{pmid}_PMC{pmc_id_clean}.pdf")
                    with open(pdf_path, 'wb') as f:
                        f.write(pdf_response.content)
                    
                    # Upload to S3 if requested
                    if upload_to_s3 and s3_bucket:
                        st.info(f"🔍 DEBUG: Attempting to upload PDF: {pdf_path}")
                        # Get prefix from parameter or config
                        prefix_to_use = s3_prefix if s3_prefix else ""
                        if not prefix_to_use:
                            try:
                                config = st.session_state.get("config", {})
                                prefix_to_use = config.get("aws", {}).get("s3_prefix", "")
                                st.info(f"🔍 DEBUG: Got prefix from config: '{prefix_to_use}'")
                            except Exception as e:
                                st.info(f"🔍 DEBUG: Error getting prefix from config: {e}")
                                prefix_to_use = ""
                        st.info(f"🔍 DEBUG: Uploading PDF with prefix: '{prefix_to_use}', bucket: {s3_bucket}")
                        success, uploaded_s3_key, error = upload_file_to_s3(
                            pdf_path, s3_bucket, None, aws_access_key_id, aws_secret_access_key, aws_region, prefix_to_use
                        )
                        if success:
                            update_s3_upload_status(pmid, uploaded_s3_key, s3_bucket)
                            st.success(f"✅ ✅ Successfully uploaded PDF to S3!")
                            st.success(f"✅ S3 Location: s3://{s3_bucket}/{uploaded_s3_key}")
                            st.info(f"📄 File: {pdf_path}")
                        else:
                            st.error(f"❌ S3 upload failed: {error}")
                            st.info(f"🔍 DEBUG: Upload error - PDF: {pdf_path}, bucket: {s3_bucket}, prefix: {prefix_to_use}")
                            st.info(f"🔍 DEBUG: File exists: {os.path.exists(pdf_path)}, File size: {os.path.getsize(pdf_path) if os.path.exists(pdf_path) else 0} bytes")
                    else:
                        if not upload_to_s3:
                            st.info(f"🔍 DEBUG: PDF upload skipped - upload_to_s3=False")
                        if not s3_bucket:
                            st.info(f"🔍 DEBUG: PDF upload skipped - no bucket configured")
                    
                    return pdf_path
                
                # If PDF not available, try to fetch full text XML from PMC
                st.info(f"🔍 DEBUG: PDF not available, trying PMC XML fetch for PMC ID: {pmc_id_clean}")
                try:
                    pmc_fetch_url = NCBI_EUTILS_BASE + "efetch.fcgi"
                    pmc_fetch_params = {
                        "db": "pmc",
                        "id": pmc_id_clean,
                        "retmode": "xml",
                        "tool": "cannabis_chat_assistant",
                        "email": "noreply@example.com"
                    }
                    
                    if api_key:
                        pmc_fetch_params["api_key"] = api_key.strip()
                    
                    st.info(f"🔍 DEBUG: Fetching PMC XML from: {pmc_fetch_url} with params: db=pmc, id={pmc_id_clean}")
                    time.sleep(delay)
                    pmc_fetch_response = requests.get(pmc_fetch_url, params=pmc_fetch_params, timeout=30)
                    
                    st.info(f"🔍 DEBUG: PMC XML response status: {pmc_fetch_response.status_code}")
                    st.info(f"🔍 DEBUG: PMC XML response size: {len(pmc_fetch_response.content)} bytes")
                    
                    if pmc_fetch_response.status_code == 200:
                        # Check if response contains actual content
                        if len(pmc_fetch_response.content) < 100:
                            st.warning(f"⚠️ PMC XML response too small ({len(pmc_fetch_response.content)} bytes), may be empty")
                            st.info(f"🔍 DEBUG: Response content preview: {pmc_fetch_response.content[:200]}")
                        else:
                            # Parse PMC XML to extract full text
                            try:
                                pmc_root = ET.fromstring(pmc_fetch_response.content)
                                st.info(f"🔍 DEBUG: Successfully parsed PMC XML")
                                full_text = extract_full_text_from_pmc_xml(pmc_root, root, pmid)
                                
                                if full_text:
                                    st.info(f"🔍 DEBUG: Extracted full text length: {len(full_text)} characters")
                                else:
                                    st.warning(f"⚠️ extract_full_text_from_pmc_xml returned None/empty")
                            except ET.ParseError as parse_err:
                                st.error(f"❌ Error parsing PMC XML: {parse_err}")
                                st.info(f"🔍 DEBUG: XML content preview: {pmc_fetch_response.content[:500]}")
                                full_text = None
                        
                        if full_text and len(full_text) > 500:  # Ensure we got substantial content, not just abstract
                            st.success(f"✅ ✅ Successfully extracted FULL TEXT article from PMC for PMID {pmid}")
                            st.info(f"🔍 DEBUG: Full text length: {len(full_text)} characters")
                            # Save as text file
                            safe_title = extract_title_for_filename(root)
                            txt_path = os.path.join(docs_dir, f"PMID_{pmid}_{safe_title}.txt")
                            with open(txt_path, 'w', encoding='utf-8') as f:
                                f.write(full_text)
                            
                            st.info(f"✅ Saved FULL TEXT article to: {txt_path}")
                            
                            # Store in database - extract metadata from PubMed XML
                            article_elem = root.find(".//PubmedArticle")
                            if article_elem is not None:
                                authors_list = []
                                for author in article_elem.findall(".//Author"):
                                    last_name_elem = author.find("LastName")
                                    first_name_elem = author.find("ForeName")
                                    if last_name_elem is not None and last_name_elem.text:
                                        last_name = last_name_elem.text
                                        first_name = first_name_elem.text if first_name_elem is not None and first_name_elem.text else ""
                                        if first_name:
                                            authors_list.append(f"{first_name} {last_name}")
                                        else:
                                            authors_list.append(last_name)
                                authors_str = ", ".join(authors_list[:5]) if authors_list else "Unknown"
                                
                                journal_elem = article_elem.find(".//Journal/Title")
                                journal_name = journal_elem.text if journal_elem is not None else "Unknown"
                                
                                pub_date_elem = article_elem.find(".//PubDate")
                                pub_date_str = "Unknown"
                                if pub_date_elem is not None:
                                    year_elem = pub_date_elem.find("Year")
                                    if year_elem is not None and year_elem.text:
                                        pub_date_str = year_elem.text
                                
                                doi_elem = article_elem.find(".//ELocationID[@EIdType='doi']")
                                doi_str = doi_elem.text if doi_elem is not None else None
                                
                                store_pmid_data(
                                    pmid=pmid,
                                    authors=authors_str,
                                    journal=journal_name,
                                    published_date=pub_date_str,
                                    doi=doi_str,
                                    pdf_path=txt_path
                                )
                            
                            # Upload to S3 automatically after download
                            if upload_to_s3 and s3_bucket:
                                st.info(f"🔍 DEBUG: Attempting to upload TXT (PMC): {txt_path}")
                                # Get prefix from parameter or config
                                prefix_to_use = s3_prefix if s3_prefix else ""
                                if not prefix_to_use:
                                    try:
                                        config = st.session_state.get("config", {})
                                        prefix_to_use = config.get("aws", {}).get("s3_prefix", "")
                                        st.info(f"🔍 DEBUG: Got prefix from config: '{prefix_to_use}'")
                                    except Exception as e:
                                        st.info(f"🔍 DEBUG: Error getting prefix from config: {e}")
                                        prefix_to_use = ""
                                st.info(f"🔍 DEBUG: Uploading TXT with prefix: '{prefix_to_use}', bucket: {s3_bucket}")
                                success, uploaded_s3_key, error = upload_file_to_s3(
                                    txt_path, s3_bucket, None, aws_access_key_id, aws_secret_access_key, aws_region, prefix_to_use
                                )
                                if success:
                                    update_s3_upload_status(pmid, uploaded_s3_key, s3_bucket)
                                    st.success(f"✅ Uploaded to S3: s3://{s3_bucket}/{uploaded_s3_key}")
                                else:
                                    st.error(f"❌ S3 upload failed: {error}")
                                    st.info(f"🔍 DEBUG: Upload error - TXT: {txt_path}, bucket: {s3_bucket}, prefix: {prefix_to_use}")
                            else:
                                if not upload_to_s3:
                                    st.info(f"🔍 DEBUG: TXT upload skipped - upload_to_s3=False")
                                if not s3_bucket:
                                    st.info(f"🔍 DEBUG: TXT upload skipped - no bucket configured")
                            
                            return txt_path
                        else:
                            st.warning(f"⚠️ Failed to extract full text from PMC XML for PMID {pmid}")
                            st.info(f"🔍 DEBUG: Will fall back to abstract extraction")
                    else:
                        st.error(f"❌ PMC XML fetch failed with status {pmc_fetch_response.status_code}")
                        st.info(f"🔍 DEBUG: Response: {pmc_fetch_response.text[:500]}")
                        st.warning(f"⚠️ Will fall back to abstract extraction")
                except Exception as e:
                    # If PMC full text fetch fails, log the error but don't silently fail
                    st.error(f"❌ Exception while fetching PMC full text: {str(e)}")
                    st.info(f"🔍 DEBUG: Exception type: {type(e).__name__}")
                    import traceback
                    st.info(f"🔍 DEBUG: Traceback: {traceback.format_exc()}")
                    st.warning(f"⚠️ Will fall back to abstract extraction")
            else:
                st.info(f"🔍 DEBUG: No PMC ID found for PMID {pmid}, will extract abstract only")
            
            # If PDF not available, extract full text/abstract and save as TXT
            article_elem = root.find(".//PubmedArticle")
            if article_elem is not None:
                # Extract title
                title_elem = article_elem.find(".//ArticleTitle")
                title = title_elem.text if title_elem is not None else f"Article_{pmid}"
                
                # Extract abstract
                abstract_parts = []
                abstract_elems = article_elem.findall(".//AbstractText")
                if abstract_elems:
                    for abs_elem in abstract_elems:
                        label = abs_elem.get("Label", "")
                        text = abs_elem.text if abs_elem.text else ""
                        # Also get any nested text
                        nested_text = "".join(abs_elem.itertext()).strip()
                        if nested_text:
                            text = nested_text
                        if label:
                            abstract_parts.append(f"{label}: {text}")
                        else:
                            abstract_parts.append(text)
                
                # Extract keywords if available
                keyword_list = []
                keyword_elems = article_elem.findall(".//Keyword")
                for kw_elem in keyword_elems:
                    if kw_elem.text:
                        keyword_list.append(kw_elem.text)
                
                # Extract MeSH terms if available
                mesh_list = []
                mesh_elems = article_elem.findall(".//MeshHeading")
                for mesh_elem in mesh_elems:
                    descriptor = mesh_elem.find(".//DescriptorName")
                    if descriptor is not None and descriptor.text:
                        mesh_list.append(descriptor.text)
                
                # Extract all text from article (including any available full text sections)
                article_text_sections = []
                # Try to find any available full text sections in PubMed record
                for section in article_elem.findall(".//Abstract"):
                    section_text = "".join(section.itertext()).strip()
                    if section_text:
                        article_text_sections.append(section_text)
                
                # Extract authors
                authors = []
                for author in article_elem.findall(".//Author"):
                    last_name_elem = author.find("LastName")
                    first_name_elem = author.find("ForeName")
                    if last_name_elem is not None and last_name_elem.text:
                        last_name = last_name_elem.text
                        first_name = first_name_elem.text if first_name_elem is not None and first_name_elem.text else ""
                        if first_name:
                            authors.append(f"{first_name} {last_name}")
                        else:
                            authors.append(last_name)
                
                # Extract journal
                journal_elem = article_elem.find(".//Journal/Title")
                journal = journal_elem.text if journal_elem is not None else "Unknown"
                
                # Extract publication date
                pub_date_elem = article_elem.find(".//PubDate")
                pub_date = "Unknown"
                if pub_date_elem is not None:
                    year_elem = pub_date_elem.find("Year")
                    month_elem = pub_date_elem.find("Month")
                    day_elem = pub_date_elem.find("Day")
                    if year_elem is not None and year_elem.text:
                        date_parts = []
                        if month_elem is not None and month_elem.text:
                            date_parts.append(month_elem.text)
                        if day_elem is not None and day_elem.text:
                            date_parts.append(day_elem.text)
                        date_parts.append(year_elem.text)
                        pub_date = " ".join(date_parts)
                
                # Extract DOI
                doi_elem = article_elem.find(".//ELocationID[@EIdType='doi']")
                doi = doi_elem.text if doi_elem is not None else None
                
                # Create comprehensive text file content
                text_content = f"Title: {title}\n\n"
                text_content += f"PMID: {pmid}\n"
                if doi:
                    text_content += f"DOI: {doi}\n"
                text_content += f"Journal: {journal}\n"
                text_content += f"Published: {pub_date}\n"
                text_content += f"Authors: {', '.join(authors[:10])}{' et al.' if len(authors) > 10 else ''}\n\n"
                
                if keyword_list:
                    text_content += f"Keywords: {', '.join(keyword_list)}\n\n"
                
                if mesh_list:
                    text_content += f"MeSH Terms: {', '.join(mesh_list[:10])}{'...' if len(mesh_list) > 10 else ''}\n\n"
                
                text_content += "=" * 80 + "\n"
                text_content += "ABSTRACT\n"
                text_content += "=" * 80 + "\n\n"
                text_content += "\n".join(abstract_parts) if abstract_parts else "No abstract available."
                text_content += "\n\n"
                
                # Add any additional text sections if available
                if article_text_sections:
                    text_content += "=" * 80 + "\n"
                    text_content += "ADDITIONAL CONTENT\n"
                    text_content += "=" * 80 + "\n\n"
                    for section in article_text_sections:
                        text_content += section + "\n\n"
                
                text_content += "\n" + "=" * 80 + "\n"
                text_content += f"Note: This is the full record available from PubMed for PMID {pmid}.\n"
                text_content += "For complete full-text article, please check if the article has a free PMC version.\n"
                text_content += f"PubMed URL: https://pubmed.ncbi.nlm.nih.gov/{pmid}/\n"
                
                # Save as text file
                # Sanitize title for filename
                safe_title = "".join(c for c in title[:50] if c.isalnum() or c in (' ', '-', '_')).strip()
                safe_title = safe_title.replace(' ', '_')
                txt_path = os.path.join(docs_dir, f"PMID_{pmid}_{safe_title}.txt")
                
                with open(txt_path, 'w', encoding='utf-8') as f:
                    f.write(text_content)
                
                # Determine content type
                if abstract_parts:
                    content_type = "Abstract + Metadata"
                    if article_text_sections:
                        content_type = "Abstract + Additional Content"
                else:
                    content_type = "Metadata Only"
                
                st.info(f"✅ Extracted {content_type} for PMID {pmid}")
                st.info(f"✅ Saved article text to: {txt_path}")
                
                # Upload to S3 automatically after download
                if upload_to_s3 and s3_bucket:
                    st.info(f"🔍 DEBUG: Attempting to upload TXT (abstract): {txt_path}")
                    # Get prefix from parameter or config
                    prefix_to_use = s3_prefix if s3_prefix else ""
                    if not prefix_to_use:
                        try:
                            config = st.session_state.get("config", {})
                            prefix_to_use = config.get("aws", {}).get("s3_prefix", "")
                            st.info(f"🔍 DEBUG: Got prefix from config: '{prefix_to_use}'")
                        except Exception as e:
                            st.info(f"🔍 DEBUG: Error getting prefix from config: {e}")
                            prefix_to_use = ""
                    st.info(f"🔍 DEBUG: Uploading TXT with prefix: '{prefix_to_use}', bucket: {s3_bucket}")
                    success, uploaded_s3_key, error = upload_file_to_s3(
                        txt_path, s3_bucket, None, aws_access_key_id, aws_secret_access_key, aws_region, prefix_to_use
                    )
                    if success:
                        update_s3_upload_status(pmid, uploaded_s3_key, s3_bucket)
                        st.success(f"✅ Uploaded to S3: s3://{s3_bucket}/{uploaded_s3_key}")
                    else:
                        st.error(f"❌ S3 upload failed: {error}")
                        st.info(f"🔍 DEBUG: Upload error - TXT: {txt_path}, bucket: {s3_bucket}, prefix: {prefix_to_use}")
                else:
                    if not upload_to_s3:
                        st.info(f"🔍 DEBUG: TXT upload skipped - upload_to_s3=False")
                    if not s3_bucket:
                        st.info(f"🔍 DEBUG: TXT upload skipped - no bucket configured")
                
                return txt_path
        
        except ET.ParseError as e:
            st.warning(f"⚠️ Error parsing XML for PMID {pmid}: {str(e)}")
            return None
        
        # If nothing worked, return None
        return None
        
    except Exception as e:
        st.warning(f"⚠️ Error fetching content for PMID {pmid}: {str(e)}")
        return None

# Initialize PubMed database on module load
init_pubmed_database()

def search_pubmed(query: str, max_results: int = 3, api_key: Optional[str] = None) -> Dict:
    """
    Search PubMed for articles and educational materials related to the query using NCBI E-utilities API.
    Returns a dictionary with search info (count, query, PMIDs) and article details.
    
    Args:
        query: User question string (will be processed to extract keywords)
        max_results: Maximum number of results to return
        api_key: Optional NCBI API key for higher rate limits (10 req/sec vs 3 req/sec)
    """
    pubmed_results = {
        "count": 0,
        "query": query,
        "pmids": [],
        "articles": []
    }
    
    try:
        # Extract keywords from user question
        pubmed_query = extract_keywords_for_pubmed(query)
        
        # Clean and validate query
        pubmed_query = (pubmed_query or "").strip()
        if not pubmed_query:
            st.warning("⚠️ Could not extract meaningful keywords from query.")
            return pubmed_results
        
        # Step 1: Search PubMed and get PMIDs
        search_url = NCBI_EUTILS_BASE + "esearch.fcgi"
        
        # Build search parameters
        search_params = {
            "db": "pubmed",
            "term": pubmed_query,
            "retmax": max_results,
            "retmode": "json",
            "tool": "cannabis_chat_assistant",
            "email": "noreply@example.com"  # NCBI recommends providing email
        }
        
        # Add API key if provided (increases rate limit from 3 to 10 req/sec)
        if api_key:
            api_key = api_key.strip()  # Remove any spaces/newlines
            if api_key:
                search_params["api_key"] = api_key
        
        # Add rate limiting delay (0.35s without API key, 0.1s with API key)
        delay = 0.1 if api_key else 0.35
        time.sleep(delay)
        
        # Make the search request
        try:
            search_response = requests.get(search_url, params=search_params, timeout=10)
        except requests.RequestException as e:
            st.error(f"⚠️ Network error while contacting PubMed: {e}")
            return pubmed_results
        
        # Check response status and show detailed error if needed
        if not search_response.ok:
            error_details = search_response.text[:500] if search_response.text else "No details available"
            st.warning(
                f"⚠️ PubMed search error: {search_response.status_code} {search_response.reason}\n\n"
                f"Query used: {pubmed_query}\n"
                f"Details from NCBI: {error_details}"
            )
            return pubmed_results
        
        search_data = search_response.json()
        
        # Store the query used
        pubmed_results["query"] = pubmed_query
        
        result = search_data.get("esearchresult", {})
        pmids = result.get("idlist", []) or []
        count = int(result.get("count", 0))
        
        pubmed_results["count"] = count
        pubmed_results["pmids"] = pmids
        
        # Debug output
        if count > 0:
            st.success(f"✅ Found {count} PubMed articles")
        
        # Step 2: Fetch full article data using efetch (XML format) to get metadata, abstracts, and PMC IDs
        # Limit to top 3 most relevant results
        if pmids:
            # Take only top 3 most relevant PMIDs
            top_pmids = pmids[:3]
            
            fetch_url = NCBI_EUTILS_BASE + "efetch.fcgi"
            fetch_params = {
                "db": "pubmed",
                "id": ",".join(top_pmids),
                "retmode": "xml",
                "rettype": "full",  # Get full record to include PMC ID for full text detection
                "tool": "cannabis_chat_assistant",
                "email": "noreply@example.com"
            }
            
            # Add API key if provided
            if api_key:
                api_key = api_key.strip()
                if api_key:
                    fetch_params["api_key"] = api_key
            
            # Add rate limiting delay (0.35s without API key, 0.1s with API key)
            delay = 0.1 if api_key else 0.35
            time.sleep(delay)
            
            # Make the fetch request
            try:
                fetch_response = requests.get(fetch_url, params=fetch_params, timeout=10)
            except requests.RequestException as e:
                st.warning(f"⚠️ Network error while fetching article details: {e}")
                return pubmed_results
            
            # Check response status
            if not fetch_response.ok:
                error_details = fetch_response.text[:500] if fetch_response.text else "No details available"
                st.warning(
                    f"⚠️ Error fetching article details: {fetch_response.status_code} {fetch_response.reason}\n"
                    f"Details: {error_details}"
                )
                return pubmed_results
            
            # Parse XML response
            try:
                root = ET.fromstring(fetch_response.content)
            except ET.ParseError as e:
                st.warning(f"⚠️ Error parsing XML response: {e}")
                return pubmed_results
            
            # Parse each article from XML
            articles_parsed = 0
            for article in root.findall(".//PubmedArticle"):
                article_data = parse_pubmed_article_xml(article)
                if article_data:
                    pmid = article_data.get("pmid")
                    
                    # Store PMID data in database (without PDF download - user will click button)
                    if pmid:
                        # Check if PMID exists, if not store metadata
                        if not pmid_exists(pmid):
                            # Store PMID data in database (without PDF path initially)
                            store_pmid_data(
                                pmid=pmid,
                                authors=article_data.get("authors", "Unknown"),
                                journal=article_data.get("journal", "Unknown"),
                                published_date=article_data.get("pub_date", "Unknown"),
                                doi=article_data.get("doi"),
                                pdf_path=None  # PDF will be downloaded when user clicks button
                            )
                    
                    pubmed_results["articles"].append(article_data)
                    articles_parsed += 1
            
            # Debug output
            if articles_parsed > 0:
                st.success(f"✅ Parsed {articles_parsed} article(s) with abstracts")
            elif len(top_pmids) > 0:
                st.warning(f"⚠️ Found {len(top_pmids)} PMIDs but couldn't parse article details")
        
    except Exception as e:
        st.warning(f"⚠️ Error processing PubMed results: {str(e)}")
        # Return empty results structure on error
        return pubmed_results
    
    return pubmed_results

def parse_pubmed_article_xml(article_element) -> Optional[Dict]:
    """
    Parse a PubMed article XML element from efetch and extract relevant information including full abstract.
    """
    try:
        article_data = {}
        
        # Extract PMID
        pmid_elem = article_element.find(".//PMID")
        if pmid_elem is not None and pmid_elem.text:
            article_data["pmid"] = pmid_elem.text
            article_data["pubmed_url"] = f"https://pubmed.ncbi.nlm.nih.gov/{pmid_elem.text}/"
        else:
            return None  # Need PMID to proceed
        
        # Extract title
        title_elem = article_element.find(".//ArticleTitle")
        if title_elem is not None and title_elem.text:
            article_data["title"] = title_elem.text
        else:
            return None  # Need title to proceed
        
        # Extract abstract - EFetch provides full abstracts
        abstract_elems = article_element.findall(".//AbstractText")
        if abstract_elems:
            abstract_parts = []
            for abs_elem in abstract_elems:
                label = abs_elem.get("Label", "")
                text = abs_elem.text if abs_elem.text else ""
                if label:
                    abstract_parts.append(f"{label}: {text}")
                else:
                    abstract_parts.append(text)
            article_data["abstract"] = " ".join(abstract_parts).strip()
        else:
            # Try alternative abstract location
            abstract_elem = article_element.find(".//Abstract")
            if abstract_elem is not None:
                abstract_text = "".join(abstract_elem.itertext()).strip()
                if abstract_text:
                    article_data["abstract"] = abstract_text
                else:
                    article_data["abstract"] = "No abstract available."
            else:
                article_data["abstract"] = "No abstract available."
        
        # Extract PMC ID to detect if full text is available
        pmc_id_elem = article_element.find(".//ArticleId[@IdType='pmc']")
        if pmc_id_elem is None:
            pmc_id_elem = article_element.find(".//ArticleIdList/ArticleId[@IdType='pmc']")
        pmc_id = pmc_id_elem.text if pmc_id_elem is not None else None
        
        # Also check for PMCID in other formats
        if not pmc_id:
            for article_id in article_element.findall(".//ArticleId"):
                id_type = article_id.get("IdType", "")
                id_text = article_id.text if article_id.text else ""
                if id_type.lower() == "pmc" or (id_text and id_text.upper().startswith("PMC")):
                    pmc_id = id_text
                    break
        
        article_data["pmc_id"] = pmc_id  # Store PMC ID if available
        article_data["has_full_text"] = bool(pmc_id)  # Flag indicating full text availability
        
        # Extract authors
        author_list = []
        for author in article_element.findall(".//Author"):
            last_name_elem = author.find("LastName")
            first_name_elem = author.find("ForeName")
            if last_name_elem is not None and last_name_elem.text:
                last_name = last_name_elem.text
                first_name = first_name_elem.text if first_name_elem is not None and first_name_elem.text else ""
                if first_name:
                    author_list.append(f"{first_name} {last_name}")
                else:
                    author_list.append(last_name)
        
        if author_list:
            article_data["authors"] = ", ".join(author_list[:5])  # Limit to first 5 authors
            if len(author_list) > 5:
                article_data["authors"] += " et al."
        else:
            article_data["authors"] = "Unknown"
        
        # Extract journal
        journal_elem = article_element.find(".//Journal/Title")
        if journal_elem is not None and journal_elem.text:
            article_data["journal"] = journal_elem.text
        else:
            article_data["journal"] = "Unknown"
        
        # Extract publication date
        pub_date_elem = article_element.find(".//PubDate")
        if pub_date_elem is not None:
            year_elem = pub_date_elem.find("Year")
            month_elem = pub_date_elem.find("Month")
            day_elem = pub_date_elem.find("Day")
            if year_elem is not None and year_elem.text:
                date_parts = []
                if month_elem is not None and month_elem.text:
                    date_parts.append(month_elem.text)
                if day_elem is not None and day_elem.text:
                    date_parts.append(day_elem.text)
                date_parts.append(year_elem.text)
                article_data["pub_date"] = " ".join(date_parts)
            else:
                article_data["pub_date"] = "Unknown"
        else:
            article_data["pub_date"] = "Unknown"
        
        # Extract DOI if available
        doi_elem = article_element.find(".//ELocationID[@EIdType='doi']")
        if doi_elem is not None and doi_elem.text:
            article_data["doi"] = doi_elem.text
        
        return article_data
        
    except Exception as e:
        return None

def parse_pubmed_summary(article_info: Dict, pmid: str) -> Optional[Dict]:
    """
    Parse a PubMed article summary from esummary JSON response and extract relevant information.
    """
    try:
        article_data = {
            "pmid": pmid,
            "pubmed_url": f"https://pubmed.ncbi.nlm.nih.gov/{pmid}/"
        }
        
        # Extract title
        title = article_info.get("title", "")
        if title:
            article_data["title"] = title
        
        # Extract abstract
        abstract = article_info.get("abstract", "")
        if abstract:
            article_data["abstract"] = abstract
        else:
            article_data["abstract"] = "No abstract available."
        
        # Extract authors - esummary returns authors as a list of dicts
        authors = article_info.get("authors", [])
        if authors and isinstance(authors, list):
            author_list = []
            for author in authors[:5]:  # Limit to first 5 authors
                if isinstance(author, dict):
                    name = author.get("name", "")
                    if not name:
                        # Try alternative format: LastName + ForeName
                        last_name = author.get("lastname", "")
                        fore_name = author.get("forename", "")
                        if last_name:
                            name = f"{fore_name} {last_name}".strip() if fore_name else last_name
                    if name:
                        author_list.append(name)
            if author_list:
                article_data["authors"] = ", ".join(author_list)
                if len(authors) > 5:
                    article_data["authors"] += " et al."
            else:
                article_data["authors"] = "Unknown"
        else:
            article_data["authors"] = "Unknown"
        
        # Extract journal
        source = article_info.get("source", "")
        if source:
            article_data["journal"] = source
        else:
            article_data["journal"] = "Unknown"
        
        # Extract publication date
        pubdate = article_info.get("pubdate", "")
        if pubdate:
            article_data["pub_date"] = pubdate
        else:
            article_data["pub_date"] = "Unknown"
        
        # Extract DOI if available
        elocationid = article_info.get("elocationid", "")
        if elocationid and isinstance(elocationid, str) and elocationid.startswith("10."):
            article_data["doi"] = elocationid
        elif isinstance(elocationid, list):
            for eid in elocationid:
                if isinstance(eid, dict) and eid.get("EIdType") == "doi":
                    article_data["doi"] = eid.get("value", "")
                    break
        
        return article_data if article_data.get("title") else None
        
    except Exception as e:
        return None

def format_summary_for_display(summary_text: str) -> str:
    """
    Format the summary text with beautiful styling for disclaimer and 4 categories.
    """
    if not summary_text:
        return ""
    
    # Split the summary into lines
    lines = summary_text.split('\n')
    
    html_parts = []
    current_section = None
    disclaimer_text = []
    collecting_disclaimer = False
    
    i = 0
    while i < len(lines):
        line = lines[i].strip()
        
        # Skip empty lines
        if not line:
            i += 1
            continue
        
        # Detect disclaimer (usually starts with "I'm not a doctor")
        if ("I'm not a doctor" in line or "I am not a doctor" in line.lower()) and not collecting_disclaimer:
            collecting_disclaimer = True
            disclaimer_text = [line]
            # Continue collecting disclaimer text until we hit a category or empty line followed by category
            i += 1
            while i < len(lines):
                next_line = lines[i].strip()
                # Stop if we hit a category emoji
                if any(emoji in next_line for emoji in ['✅', '⚠️', '🦵', '🎯']):
                    i -= 1  # Back up to process the category header
                    break
                elif next_line:
                    disclaimer_text.append(next_line)
                elif not next_line and i + 1 < len(lines):
                    # Check if next non-empty line is a category
                    peek_idx = i + 1
                    while peek_idx < len(lines) and not lines[peek_idx].strip():
                        peek_idx += 1
                    if peek_idx < len(lines) and any(emoji in lines[peek_idx] for emoji in ['✅', '⚠️', '🦵', '🎯']):
                        i = peek_idx - 1  # Position to process category next
                        break
                i += 1
            
            # Format disclaimer box
            disclaimer_html = '<div style="background-color: #e3f2fd; padding: 15px; border-radius: 8px; margin: 15px 0 20px 0; border-left: 4px solid #2196f3;">'
            disclaimer_html += '<p style="margin: 0; color: #1565c0; font-size: 15px; line-height: 1.6;">' + ' '.join(disclaimer_text) + '</p>'
            disclaimer_html += '</div>'
            html_parts.append(disclaimer_html)
            collecting_disclaimer = False
            i += 1
            continue
        
        # Detect category headers
        if line.startswith('✅'):
            current_section = 'research'
            html_parts.append(f'<div style="margin-top: 24px; margin-bottom: 12px;"><h4 style="color: #2e7d32; margin: 0; font-size: 19px; font-weight: 700;">{line}</h4></div>')
        elif line.startswith('⚠️'):
            current_section = 'legal'
            html_parts.append(f'<div style="margin-top: 24px; margin-bottom: 12px;"><h4 style="color: #f57c00; margin: 0; font-size: 19px; font-weight: 700;">{line}</h4></div>')
        elif line.startswith('🦵'):
            current_section = 'products'
            html_parts.append(f'<div style="margin-top: 24px; margin-bottom: 12px;"><h4 style="color: #1976d2; margin: 0; font-size: 19px; font-weight: 700;">{line}</h4></div>')
        elif line.startswith('🎯'):
            current_section = 'suggestion'
            html_parts.append(f'<div style="margin-top: 24px; margin-bottom: 12px;"><h4 style="color: #7b1fa2; margin: 0; font-size: 19px; font-weight: 700;">{line}</h4></div>')
        
        # Handle bullet points
        elif (line.startswith('-') or line.startswith('•')) and current_section:
            # Remove the bullet and format as list item
            content = line.lstrip('- •').strip()
            if content:
                # Check for bold text within bullet (e.g., "**Oral forms:**")
                if '**' in content:
                    parts = content.split('**')
                    formatted_content = ''
                    for j, part in enumerate(parts):
                        if j % 2 == 1:  # Odd indices are bold
                            formatted_content += f'<strong style="color: #1976d2;">{part}</strong>'
                        else:
                            formatted_content += part
                    html_parts.append(f'<div style="margin-left: 20px; margin-bottom: 10px; padding-left: 12px; border-left: 3px solid #e0e0e0;"><p style="margin: 0; color: #424242; line-height: 1.7; font-size: 15px;">{formatted_content}</p></div>')
                else:
                    html_parts.append(f'<div style="margin-left: 20px; margin-bottom: 10px; padding-left: 12px; border-left: 3px solid #e0e0e0;"><p style="margin: 0; color: #424242; line-height: 1.7; font-size: 15px;">{content}</p></div>')
        
        # Handle bold text (section headers within categories)
        elif line.startswith('**') and line.endswith('**') and current_section:
            bold_text = line.strip('*')
            html_parts.append(f'<div style="margin-top: 14px; margin-bottom: 8px; margin-left: 4px;"><strong style="color: #1976d2; font-size: 16px;">{bold_text}</strong></div>')
        
        # Regular paragraph content within a section
        elif line and current_section and not line.startswith('-') and not line.startswith('•'):
            # Check if line contains links (Source [X] citations)
            html_parts.append(f'<div style="margin-bottom: 10px;"><p style="margin: 0; color: #424242; line-height: 1.7; font-size: 15px;">{line}</p></div>')
        
        # Content before any section (shouldn't happen with our format, but handle it)
        elif line and not current_section and not collecting_disclaimer:
            html_parts.append(f'<div style="margin-bottom: 8px;"><p style="margin: 0; color: #424242; line-height: 1.6;">{line}</p></div>')
        
        i += 1
    
    # If no formatting was applied, return original text with basic styling
    if len(html_parts) == 0:
        return f'<div style="color: #424242; line-height: 1.6; padding: 15px;">{summary_text.replace(chr(10), "<br>")}</div>'
    
    return '<div style="background-color: #ffffff; padding: 25px; border-radius: 12px; margin: 15px 0; border: 1px solid #e0e0e0; box-shadow: 0 2px 4px rgba(0,0,0,0.1);">' + ''.join(html_parts) + '</div>'


def get_first_sentences(text: str, num_sentences: int = 3) -> str:
    """
    Extract the first N sentences from text.
    Handles common abbreviations and edge cases.
    """
    if not text or text == "No abstract available.":
        return text
    
    # Common abbreviations that shouldn't end sentences
    abbreviations = ['dr', 'mr', 'mrs', 'ms', 'prof', 'vs', 'etc', 'e.g', 'i.e', 'cf', 'fig', 'no', 'vol', 'pp']
    
    # Split by sentence endings (period, exclamation, question mark followed by space or end)
    # Use a more sophisticated regex that checks for space after punctuation
    sentences = re.split(r'(?<=[.!?])\s+(?=[A-Z])', text)
    
    # If the simple split didn't work well, try a simpler approach
    if len(sentences) == 1:
        sentences = re.split(r'(?<=[.!?])\s+', text)
    
    # Filter out empty sentences and clean them
    sentences = [s.strip() for s in sentences if s.strip()]
    
    # If we have 3 or fewer sentences, return the full text
    if len(sentences) <= num_sentences:
        return text
    
    # Return first N sentences joined with spaces
    result = ' '.join(sentences[:num_sentences])
    
    # Ensure it ends properly (add period if needed, but don't duplicate)
    if not result.rstrip().endswith(('.', '!', '?')):
        result += '.'
    
    return result + '...'


def format_pubmed_results(pubmed_results: Dict) -> str:
    """
    Format PubMed search results for display in the UI with expandable summaries.
    """
    if not pubmed_results:
        return ""
    
    # Check if we have articles even if count is 0 (shouldn't happen but be safe)
    articles = pubmed_results.get("articles", [])
    count = pubmed_results.get("count", 0)
    
    if count == 0 and len(articles) == 0:
        return ""
    
    formatted = f"""
### 📚 PubMed Research Articles

**Search Query:** `{pubmed_results['query']}`  
**Total Results:** {pubmed_results['count']} articles found  
**Displayed:** {len(pubmed_results['articles'])} articles

**PubMed IDs (PMIDs):** {', '.join(pubmed_results['pmids'][:10])}{'...' if len(pubmed_results['pmids']) > 10 else ''}

---
"""
    
    for idx, article in enumerate(pubmed_results['articles'], 1):
        abstract = article.get('abstract', 'No abstract available.')
        full_abstract = abstract
        preview_abstract = get_first_sentences(abstract, 3)
        
        # Create unique ID for this article's expand/collapse
        article_id = f"pubmed_{article.get('pmid', idx)}"
        
        formatted += f"""
**{idx}. {article.get('title', 'No title')}**

- **Authors:** {article.get('authors', 'Unknown')}
- **Journal:** {article.get('journal', 'Unknown')}
- **Published:** {article.get('pub_date', 'Unknown')}
- **PMID:** <a href="{article.get('pubmed_url', '#')}" target="_blank" rel="noopener noreferrer">{article.get('pmid', 'N/A')}</a>
"""
        if article.get('doi'):
            formatted += f"- **DOI:** {article['doi']}\n"
        
        # Add expandable summary section
        # Show expand option if preview was truncated (ends with '...') or if full abstract is significantly longer
        should_show_expand = abstract != 'No abstract available.' and (preview_abstract.endswith('...') or len(full_abstract) > len(preview_abstract) + 50)
        
        if should_show_expand:
            # Escape HTML special characters in abstract text
            preview_escaped = preview_abstract.replace('&', '&amp;').replace('<', '&lt;').replace('>', '&gt;').replace('"', '&quot;')
            full_escaped = full_abstract.replace('&', '&amp;').replace('<', '&lt;').replace('>', '&gt;').replace('"', '&quot;')
            
            formatted += f"""
**Summary:**
<div style="margin-top: 8px; margin-bottom: 8px;">
{preview_escaped}
</div>
<details>
<summary style="cursor: pointer; color: #1f77b4; font-weight: 500; margin-top: 8px; display: inline-block;">▶ Expand Summary</summary>
<div style="margin-top: 8px; padding-left: 10px; border-left: 2px solid #1f77b4;">
{full_escaped}
</div>
</details>
"""
        else:
            formatted += f"\n**Summary:**\n{abstract}\n"
        
        formatted += "\n---\n"
    
    return formatted

def resize_image(img: Image.Image, target_size: Tuple[int, int] = (300, 300)) -> Image.Image:
    """
    Resize image to target size while maintaining aspect ratio.
    """
    img.thumbnail(target_size, Image.Resampling.LANCZOS)
    
    # Create a new image with target size and paste resized image centered
    new_img = Image.new('RGB', target_size, (255, 255, 255))
    img_width, img_height = img.size
    x_offset = (target_size[0] - img_width) // 2
    y_offset = (target_size[1] - img_height) // 2
    new_img.paste(img, (x_offset, y_offset))
    
    return new_img

def detect_product_type(title: str, query: str = "") -> str:
    """
    Detect product type from title and query to determine default image.
    Returns: 'oil', 'cream', 'flower', 'edible', 'vape', 'concentrate', 'default'
    """
    text = (title + " " + query).lower()
    
    if any(word in text for word in ['oil', 'tincture', 'dropper', 'bottle']):
        return 'oil'
    elif any(word in text for word in ['cream', 'topical', 'lotion', 'balm', 'salve']):
        return 'cream'
    elif any(word in text for word in ['flower', 'bud', 'weed', 'marijuana', 'cannabis flower']):
        return 'flower'
    elif any(word in text for word in ['edible', 'gummy', 'cookie', 'chocolate', 'brownie', 'candy']):
        return 'edible'
    elif any(word in text for word in ['vape', 'cartridge', 'pen', 'vaporizer']):
        return 'vape'
    elif any(word in text for word in ['concentrate', 'wax', 'shatter', 'rosin', 'hash', 'kief']):
        return 'concentrate'
    else:
        return 'default'

def get_default_image(product_type: str) -> Image.Image:
    """
    Generate a default placeholder image based on product type.
    """
    # Create a simple colored placeholder image
    colors = {
        'oil': (139, 195, 74),      # Green for oil
        'cream': (255, 193, 7),      # Yellow for cream
        'flower': (76, 175, 80),    # Green for flower
        'edible': (255, 152, 0),    # Orange for edible
        'vape': (156, 39, 176),     # Purple for vape
        'concentrate': (121, 85, 72), # Brown for concentrate
        'default': (158, 158, 158)   # Gray for default
    }
    
    color = colors.get(product_type, colors['default'])
    img = Image.new('RGB', (300, 300), color)
    
    # Add text overlay (optional - can be enhanced)
    draw = ImageDraw.Draw(img)
    try:
        # Try to use a default font
        font = ImageFont.load_default()
    except:
        font = None
    
    text = product_type.upper() if product_type != 'default' else 'CANNABIS'
    bbox = draw.textbbox((0, 0), text, font=font) if font else (0, 0, 100, 20)
    text_width = bbox[2] - bbox[0]
    text_height = bbox[3] - bbox[1]
    position = ((300 - text_width) // 2, (300 - text_height) // 2)
    
    draw.text(position, text, fill=(255, 255, 255), font=font)
    
    return img

def load_image_from_url(url: str, resize_to: Tuple[int, int] = (300, 300)) -> Optional[Image.Image]:
    """
    Load image from URL safely and resize to consistent size.
    """
    try:
        response = requests.get(url, timeout=5, headers={'User-Agent': 'Mozilla/5.0'})
        if response.status_code == 200:
            img = Image.open(BytesIO(response.content))
            # Convert to RGB if necessary
            if img.mode != 'RGB':
                img = img.convert('RGB')
            # Resize to consistent size
            img = resize_image(img, resize_to)
            return img
    except Exception as e:
        st.info(f"Could not load image from {url}: {str(e)}")
    return None

def format_search_results(text_results: List[Dict], image_results: List[Dict] = None) -> str:
    """
    Format search results with proper citations for inclusion in the prompt.
    Includes reputation indicators and explicit URLs.
    """
    formatted = ""
    
    if text_results:
        formatted += "\n\n**Web Search Results with Citations (MUST INCLUDE URLs IN YOUR RESPONSE):**\n"
        for i, result in enumerate(text_results, 1):
            title = result.get('title', 'No title')
            body = result.get('body', 'No description')
            url = result.get('href', 'Unknown')
            
            # Add reputation indicator
            reputation_badge = "✅ Reputable Source" if is_reputable_source(url) else ""
            
            formatted += f"\n[{i}] **{title}** {reputation_badge}\n"
            formatted += f"   {body}\n"
            formatted += f"   📎 FULL URL: {url}\n"
            formatted += f"   IMPORTANT: When citing this source, you MUST include the URL: {url}\n"
        
        formatted += "\n**CRITICAL INSTRUCTION:** When you cite these sources in your response, you MUST include the full URL for each citation. Format: [1] Title (URL: https://...) or [1] Title - https://..."
    
    if image_results:
        formatted += f"\n\n**Found {len(image_results)} product images with URLs:**\n"
        for i, img_result in enumerate(image_results, 1):
            img_url = img_result.get('image', '')
            source_url = img_result.get('url', img_result.get('href', ''))
            title = img_result.get('title', f'Image {i}')
            if img_url:
                formatted += f"\n[{i}] {title}\n"
                formatted += f"   Image URL: {img_url}\n"
                if source_url:
                    formatted += f"   Source URL: {source_url}\n"
    
    return formatted

def generate_image_title(full_title: str) -> str:
    """
    Generate a short title (3-4 keywords) from a full image title.
    """
    if not full_title:
        return "Product"
    
    # Remove common words and extract key terms
    words = full_title.lower().split()
    stop_words = {"the", "a", "an", "and", "or", "but", "in", "on", "at", "to", "for", "of", "with", "from", "by", "as", "is", "are", "was", "were", "be", "been", "being", "have", "has", "had", "do", "does", "did", "will", "would", "should", "could", "can", "may", "might", "must", "shall", "how", "to", "guide", "ultimate"}
    
    # Extract meaningful keywords (3-4 words)
    keywords = [w for w in words if w not in stop_words and len(w) > 2][:4]
    
    if keywords:
        title = " ".join(keywords).title()
        # Limit to reasonable length
        if len(title) > 50:
            title = " ".join(keywords[:3]).title()
        return title
    else:
        # Fallback: use first few words
        return " ".join(words[:3]).title() if len(words) >= 3 else full_title[:40]

def generate_keyword_title(user_question: str, api_key: str) -> str:
    """
    Generate a concise keyword-based title from the user question using LLM.
    Returns a clear adjective + noun phrase like "Chronic Shoulder Pain" or "Knee Pain".
    """
    if not api_key:
        # Fallback: extract simple keyword with proper format
        words = user_question.lower().split()
        # Try to find pain/condition keywords
        condition_keywords = ['pain', 'knee', 'back', 'shoulder', 'chronic', 'arthritis', 'sleep', 'anxiety', 'inflammation']
        found = [w for w in words if w in condition_keywords]
        if found:
            # Format as adjective + noun (e.g., "Chronic Pain", "Shoulder Pain")
            if 'chronic' in found:
                pain_word = next((w for w in found if w != 'chronic'), 'pain')
                return f"Chronic {pain_word.title()}"
            elif len(found) >= 2:
                return ' '.join(found[:2]).title()
            else:
                return found[0].title()
        return user_question[:50] if len(user_question) > 50 else user_question
    
    try:
        client = OpenAI(api_key=api_key)
        
        keyword_prompt = f"""Extract 2-3 most relevant keywords (3-4 words maximum) from this cannabis-related question.

Question: {user_question}

Return ONLY 2-3 relevant keywords in the format: [Keyword1] [Keyword2] [Keyword3]. Examples:
- "I am looking for a cannabis product for knee pain" → "Knee Pain Cannabis"
- "What cannabis strains help with chronic back pain?" → "Chronic Back Pain"
- "Cannabis for chronic shoulder pain" → "Chronic Shoulder Pain"
- "Best CBD products for arthritis" → "CBD Arthritis Products"
- "Cannabis for sleep relaxation" → "Sleep Relaxation Cannabis"
- "Need cannabis product recommendations for relaxation" → "Cannabis Relaxation Products"

IMPORTANT: 
- Extract 2-3 most relevant keywords (3-4 words total maximum)
- No dashes, no special characters
- Capitalize each word properly
- Return only the keywords, nothing else

Keywords:"""
        
        api_params = {
            "model": "gpt-5-mini",
            "messages": [
                {"role": "system", "content": "You extract concise, clear keywords from cannabis-related questions. Always return proper adjective + noun phrases with no dashes or special characters. Return only the keyword/phrase, no explanations."},
                {"role": "user", "content": keyword_prompt}
            ],
            "temperature": 0.2,
            "max_completion_tokens": 20
        }
        
        keyword_response = client.chat.completions.create(**api_params)
        keyword = keyword_response.choices[0].message.content.strip()
        
        # Clean up the keyword (remove quotes, dashes, extra text)
        keyword = keyword.strip('"\'')
        # Remove any dashes and replace with spaces
        keyword = keyword.replace('-', ' ').replace('—', ' ').replace('–', ' ')
        # Remove extra whitespace
        keyword = ' '.join(keyword.split())
        # Capitalize properly
        keyword = ' '.join(word.capitalize() for word in keyword.split())
        
        if len(keyword) > 50:
            keyword = keyword[:47] + "..."
        
        return keyword if keyword else user_question[:50]
        
    except Exception as e:
        # Fallback: extract simple keyword with proper format
        words = user_question.lower().split()
        condition_keywords = ['pain', 'knee', 'back', 'shoulder', 'chronic', 'arthritis', 'sleep', 'anxiety', 'inflammation']
        found = [w for w in words if w in condition_keywords]
        if found:
            # Format as adjective + noun (e.g., "Chronic Pain", "Shoulder Pain")
            if 'chronic' in found:
                pain_word = next((w for w in found if w != 'chronic' and w != 'pain'), 'pain')
                if pain_word == 'pain':
                    return "Chronic Pain"
                return f"Chronic {pain_word.title()} Pain"
            elif len(found) >= 2:
                return ' '.join(found[:2]).title()
            else:
                return found[0].title()
        return user_question[:50] if len(user_question) > 50 else user_question

def summarize_top_sources(text_results: List[Dict], api_key: str, user_question: str, pubmed_results: Optional[Dict] = None) -> Tuple[str, List[Dict]]:
    """
    Use GPT-5-mini to summarize information from top 2 sources and PubMed articles.
    Returns (summary_text, top_sources_used)
    """
    if not text_results or len(text_results) < 1:
        return "", []
    
    if not api_key:
        return "", []
    
    try:
        # Get top 2 sources (prioritize reputable sources)
        top_sources = prioritize_reputable_sources(text_results)[:2]
        
        if not top_sources:
            return "", []
        
        client = OpenAI(api_key=api_key)
        
        # Build summary prompt with web sources
        sources_text = ""
        source_num = 1
        for result in top_sources:
            title = result.get('title', 'No title')
            body = result.get('body', 'No description')
            url = result.get('href', '')
            sources_text += f"\n[Source {source_num}] {title}\n{body}\nURL: {url}\n"
            source_num += 1
        
        # Add PubMed articles if available
        pubmed_source_num = source_num
        if pubmed_results and pubmed_results.get("articles"):
            sources_text += "\n--- PubMed Research Articles ---\n"
            for article in pubmed_results["articles"][:2]:  # Top 2 PubMed articles
                title = article.get('title', 'No title')
                abstract = article.get('abstract', 'No abstract')
                pmid = article.get('pmid', '')
                pubmed_url = article.get('pubmed_url', '')
                sources_text += f"\n[Source {pubmed_source_num}] {title}\nAbstract: {abstract[:500]}...\nPMID: {pmid}\nURL: {pubmed_url}\n"
                pubmed_source_num += 1
        
        summary_prompt = f"""Based on the user's question about cannabis and the following sources (including PubMed research articles), provide a comprehensive summary structured into 4 categories with a disclaimer.

User Question: {user_question}

Sources:
{sources_text}

IMPORTANT: Structure your response EXACTLY as follows:

1. Start with a disclaimer: "I'm not a doctor, but I can share some information about [topic], along with important safety and legal considerations, so you can discuss it with your healthcare provider."

2. Then organize the information into these 4 categories:

✅ What the research says
- Summarize key research findings from PubMed articles and web sources
- Include specific studies, evidence levels, and limitations
- Use "Source [X]" format when citing sources (e.g., "Source [1]", "Source [2]")
- Be honest about evidence quality and limitations

⚠️ Legal & safety considerations
- Federal and state legal status (hemp-derived CBD vs medical/recreational cannabis)
- Safety concerns: dosing, quality control, drug interactions
- Regulatory status (FDA approvals, unregulated products)
- State-specific legal variations

🦵 What you might look for (if legal and you choose to proceed)
- Product types relevant to the user's question (topicals, oral, etc.)
- Specific ingredients or formulations to consider
- Quality indicators (COA, lab testing, reputable brands)
- Dosing considerations and starting points

🎯 My suggestion for you
- Personalized, practical recommendations based on the user's specific question
- Step-by-step guidance (consult healthcare provider, choose formulation, select products, start low, monitor)
- When to consider alternatives or other treatments

CRITICAL REQUIREMENTS:
- Use the exact emoji and category headers shown above (✅ ⚠️ 🦵 🎯)
- Include citations using "Source [X]" format for all sources referenced
- Be balanced, evidence-based, and emphasize consulting healthcare providers
- Keep each section informative but concise
- Focus on actionable, safe guidance"""
        
        # Use GPT-5-mini for summarization
        api_params = {
            "model": "gpt-5-mini",
            "messages": [
                {"role": "system", "content": "You are a helpful assistant that summarizes information from cannabis-related sources. Provide structured summaries with disclaimers, research findings, legal/safety considerations, product guidance, and personalized recommendations. Always emphasize consulting healthcare providers."},
                {"role": "user", "content": summary_prompt}
            ],
            "temperature": 0.5,
            "max_completion_tokens": 800
        }
        
        summary_response = client.chat.completions.create(**api_params)
        summary = summary_response.choices[0].message.content
        
        # Add PubMed articles to top_sources for citation linking
        sources_with_pubmed = list(top_sources)
        if pubmed_results and pubmed_results.get("articles"):
            for article in pubmed_results["articles"][:2]:  # Top 2 PubMed articles
                # Convert article to source format for consistency
                pubmed_source = {
                    "title": article.get("title", ""),
                    "href": article.get("pubmed_url", ""),
                    "pmid": article.get("pmid", ""),
                    "is_pubmed": True
                }
                sources_with_pubmed.append(pubmed_source)
        
        return (summary.strip() if summary else "", sources_with_pubmed)
        
    except Exception as e:
        # Log error but don't show to user - summary is optional
        error_msg = str(e).lower()
        if "model" in error_msg and ("not found" in error_msg or "invalid" in error_msg):
            # GPT-5-mini might not be available, try gpt-4o-mini as fallback
            try:
                api_params["model"] = "gpt-4o-mini"
                api_params["max_tokens"] = 800  # Use max_tokens for older models
                del api_params["max_completion_tokens"]
                summary_response = client.chat.completions.create(**api_params)
                summary = summary_response.choices[0].message.content
                return (summary.strip() if summary else "", top_sources)
            except:
                return "", []
        return "", []

def format_citations_for_display(text_results: List[Dict]) -> str:
    """
    Format citations for display in the UI with named links (not full URLs) that open in new tabs.
    Shows all reputable sources and only top 3 additional sources.
    """
    if not text_results:
        return ""
    
    # Separate reputable and other sources
    reputable_sources = []
    other_sources = []
    
    for result in text_results:
        if is_reputable_source(result.get('href', '')):
            reputable_sources.append(result)
        else:
            other_sources.append(result)
    
    citations_html = "\n\n---\n\n"
    
    # Display reputable sources first (all of them)
    if reputable_sources:
        citations_html += "**✅ Reputable Sources:**\n\n"
        for i, result in enumerate(reputable_sources, 1):
            title = result.get('title', 'No title')
            url = result.get('href', '')
            if url:
                # Use named link (title only, no full URL displayed)
                citations_html += f'{i}. <a href="{url}" target="_blank" rel="noopener noreferrer">{title}</a> <span style="color: green;">✅</span><br>\n'
            else:
                citations_html += f"{i}. {title} ✅<br>\n"
        citations_html += "\n"
    
    # Display only top 3 additional sources
    if other_sources:
        if reputable_sources:
            citations_html += "**📚 Additional Sources (Top 3):**\n\n"
        else:
            citations_html += "**📚 Sources:**\n\n"
        
        # Limit to top 3
        top_3_others = other_sources[:3]
        start_num = len(reputable_sources) + 1
        for i, result in enumerate(top_3_others, start=start_num):
            title = result.get('title', 'No title')
            url = result.get('href', '')
            if url:
                # Use named link (title only, no full URL displayed)
                citations_html += f'{i}. <a href="{url}" target="_blank" rel="noopener noreferrer">{title}</a><br>\n'
            else:
                citations_html += f"{i}. {title}<br>\n"
    
    return citations_html

def get_response(user_question: str, api_key: str, use_web_search: bool = True, use_pubmed_search: bool = True) -> Tuple[str, List[Dict], List[Dict], str, List[Dict], str, Optional[Dict]]:
    """
    Get response from OpenAI with web search and cannabis guardrails.
    Returns (response_text, text_results, image_results, summary, top_sources_for_summary, keyword_title, pubmed_results)
    """
    config = st.session_state.config
    
    # Check if OpenAI is enabled
    if not api_key or not config["openai"]["enabled"]:
        return "⚠️ OpenAI API key is required. Please set your OpenAI API key in the sidebar or config.json file.", [], [], "", [], "", None
    
    # Check cannabis relevance
    is_relevant, guardrail_message = check_cannabis_relevance(user_question)
    if not is_relevant:
        return guardrail_message, [], [], "", [], "", None
    
    try:
        client = OpenAI(api_key=api_key)
        
        # Perform web search if enabled
        text_results = []
        image_results = []
        search_context = ""
        pubmed_results = None
        
        if use_web_search and config["features"]["web_search"]["enabled"]:
            text_results, image_results = perform_web_search(user_question)
            search_context = format_search_results(text_results, image_results)
        
        # Perform PubMed search for research articles (independent of web search)
        if use_pubmed_search and config["features"]["pubmed_search"]["enabled"]:
            pubmed_api_key = st.session_state.get("pubmed_api_key", "")
            # PubMed search works without API key but with rate limits
            pubmed_results = search_pubmed(user_question, max_results=3, api_key=pubmed_api_key if pubmed_api_key else None)
        
        # Build system prompt with cannabis guardrails and citation requirements
        system_prompt = """You are a knowledgeable cannabis assistant. Your role is to provide accurate, helpful information about:
- Cannabis products (flower, edibles, concentrates, tinctures, topicals, etc.)
- CBD and THC products and their effects
- Cannabis strains (indica, sativa, hybrid)
- Cultivation and growing techniques
- Medical and recreational cannabis use
- Cannabis-related laws and regulations (when appropriate)
- Terpenes and cannabinoids
- Consumption methods and safety

IMPORTANT GUARDRAILS:
- Only answer questions related to cannabis, CBD, THC, hemp, and related products
- If asked about non-cannabis topics, politely redirect to cannabis-related questions
- Provide factual, educational information
- Do not provide medical advice - recommend consulting healthcare professionals
- Be respectful and professional

CITATION REQUIREMENTS:
- When using web search results, ALWAYS cite sources using [1], [2], [3] format
- Prioritize information from reputable sources (marked with ✅ Reputable Source)
- Reference the numbered sources from the search results provided
- Include specific product names, brands, or strain names when available
- For product recommendations, mention that images are available
- Use named citations: [1] Source Title (links are provided below for verification)
- Example citation format: "According to [1] Leafly, cannabis can help with pain management."
- Users can click on source titles below to open links in new tabs

Use the provided web search results and summary to enhance your response with current information. Always cite your sources with numbered references."""
        
        # Get summary from top 2 sources using GPT-5-mini
        summary = ""
        top_sources_for_summary = []
        # Generate keyword-based title (always generate, even without search results)
        keyword_title = generate_keyword_title(user_question, api_key)
        if text_results and len(text_results) >= 1:
            summary, top_sources_for_summary = summarize_top_sources(text_results, api_key, user_question)
        
        # Build user message with search context
        user_message = user_question
        if search_context:
            user_message += search_context
        
        # Add summary if available
        if summary:
            user_message += f"\n\n**Summary from top sources:**\n{summary}"
        
        messages = [
            {"role": "system", "content": system_prompt},
            *[{"role": msg["role"], "content": msg["content"]} for msg in st.session_state.messages[-6:]],  # Include recent context
            {"role": "user", "content": user_message}
        ]
        
        # GPT-5 models use max_completion_tokens instead of max_tokens
        model_name = st.session_state.model
        is_gpt5_model = model_name.startswith("gpt-5")
        
        api_params = {
            "model": model_name,
            "messages": messages,
            "temperature": 0.7,
        }
        
        # Use max_completion_tokens for GPT-5 models, max_tokens for older models
        if is_gpt5_model:
            api_params["max_completion_tokens"] = 1000
        else:
            api_params["max_tokens"] = 1000
        
        response = client.chat.completions.create(**api_params)
        return response.choices[0].message.content, text_results, image_results, summary, top_sources_for_summary, keyword_title, pubmed_results
    except Exception as e:
        error_msg = str(e)
        # Provide helpful error messages
        if "max_tokens" in error_msg or "max_completion_tokens" in error_msg:
            return f"❌ API Error: Parameter issue. Please check model compatibility. Error: {error_msg}", [], [], "", [], "", None
        elif "model" in error_msg.lower() and ("not found" in error_msg.lower() or "invalid" in error_msg.lower()):
            return f"❌ Model Error: The model '{st.session_state.model}' may not be available. Please try a different model or check OpenAI API status.", [], [], "", [], "", None
        elif "rate limit" in error_msg.lower() or "429" in error_msg:
            return f"❌ Rate Limit: Too many requests. Please wait a moment and try again.", [], [], "", [], "", None
        elif "authentication" in error_msg.lower() or "api key" in error_msg.lower():
            return f"❌ Authentication Error: Please check your OpenAI API key in the sidebar.", [], [], "", [], "", None
        else:
            return f"❌ Error: {error_msg}", [], [], "", [], "", None

# Sidebar for configuration
with st.sidebar:
    st.header("⚙️ Configuration")
    
    # Feature Status
    st.markdown("### 📊 Feature Status")
    config = st.session_state.config
    
    openai_status = "✅ Enabled" if config["openai"]["enabled"] else "❌ Disabled (API key missing)"
    pubmed_status = "✅ Enabled" if config["pubmed"]["enabled"] else "⚠️ Limited (no API key - 3 req/sec)"
    aws_status = "✅ Enabled" if config["aws"]["enabled"] else "❌ Disabled (credentials missing)"
    
    st.markdown(f"- **OpenAI:** {openai_status}")
    st.markdown(f"- **PubMed:** {pubmed_status}")
    st.markdown(f"- **AWS S3:** {aws_status}")
    
    st.divider()
    
    # OpenAI API Key
    st.markdown("### 🤖 OpenAI")
    api_key_input = st.text_input(
        "OpenAI API Key",
        value=st.session_state.api_key,
        type="password",
        help="Enter your OpenAI API key (or set in config.json)"
    )
    if api_key_input != st.session_state.api_key:
        st.session_state.api_key = api_key_input
        st.session_state.config["openai"]["api_key"] = api_key_input
        st.session_state.config["openai"]["enabled"] = bool(api_key_input)
        save_config(st.session_state.config)
    
    # PubMed API Key
    st.markdown("### 📚 PubMed")
    if "pubmed_api_key" not in st.session_state:
        st.session_state.pubmed_api_key = config["pubmed"]["api_key"]
    
    pubmed_api_key_input = st.text_input(
        "PubMed API Key (Optional)",
        value=st.session_state.pubmed_api_key,
        type="password",
        help="Optional: Enter your NCBI API key for higher rate limits (10 req/sec vs 3 req/sec). Get one at https://www.ncbi.nlm.nih.gov/account/settings/"
    )
    if pubmed_api_key_input != st.session_state.pubmed_api_key:
        st.session_state.pubmed_api_key = pubmed_api_key_input
        st.session_state.config["pubmed"]["api_key"] = pubmed_api_key_input
        st.session_state.config["pubmed"]["enabled"] = bool(pubmed_api_key_input)
        save_config(st.session_state.config)
    
    st.divider()
    
    # S3 Configuration
    st.markdown("### ☁️ AWS S3 Configuration")
    
    if "s3_bucket" not in st.session_state:
        st.session_state.s3_bucket = config["aws"]["s3_bucket"]
    
    if "aws_access_key_id" not in st.session_state:
        st.session_state.aws_access_key_id = config["aws"]["access_key_id"]
    
    if "aws_secret_access_key" not in st.session_state:
        st.session_state.aws_secret_access_key = config["aws"]["secret_access_key"]
    
    if "aws_region" not in st.session_state:
        st.session_state.aws_region = config["aws"]["region"]
    
    if "auto_upload_to_s3" not in st.session_state:
        st.session_state.auto_upload_to_s3 = config["features"]["auto_upload_to_s3"]["enabled"]
    
    s3_bucket_input = st.text_input(
        "S3 Bucket Name",
        value=st.session_state.s3_bucket,
        help="Enter your S3 bucket name for storing PubMed articles (or set in config.json)"
    )
    if s3_bucket_input != st.session_state.s3_bucket:
        st.session_state.s3_bucket = s3_bucket_input
        st.session_state.config["aws"]["s3_bucket"] = s3_bucket_input
        # Update enabled status
        st.session_state.config["aws"]["enabled"] = bool(
            st.session_state.config["aws"]["access_key_id"] and 
            st.session_state.config["aws"]["secret_access_key"] and 
            s3_bucket_input
        )
        save_config(st.session_state.config)
    
    aws_access_key_input = st.text_input(
        "AWS Access Key ID",
        value=st.session_state.aws_access_key_id,
        type="password",
        help="Optional: Enter AWS access key ID (or set in config.json). Can also use environment variables or IAM role."
    )
    if aws_access_key_input != st.session_state.aws_access_key_id:
        st.session_state.aws_access_key_id = aws_access_key_input
        st.session_state.config["aws"]["access_key_id"] = aws_access_key_input
        # Update enabled status
        st.session_state.config["aws"]["enabled"] = bool(
            aws_access_key_input and 
            st.session_state.config["aws"]["secret_access_key"] and 
            st.session_state.config["aws"]["s3_bucket"]
        )
        save_config(st.session_state.config)
    
    aws_secret_key_input = st.text_input(
        "AWS Secret Access Key",
        value=st.session_state.aws_secret_access_key,
        type="password",
        help="Optional: Enter AWS secret access key (or set in config.json). Can also use environment variables or IAM role."
    )
    if aws_secret_key_input != st.session_state.aws_secret_access_key:
        st.session_state.aws_secret_access_key = aws_secret_key_input
        st.session_state.config["aws"]["secret_access_key"] = aws_secret_key_input
        # Update enabled status
        st.session_state.config["aws"]["enabled"] = bool(
            st.session_state.config["aws"]["access_key_id"] and 
            aws_secret_key_input and 
            st.session_state.config["aws"]["s3_bucket"]
        )
        save_config(st.session_state.config)
    
    aws_region_input = st.text_input(
        "AWS Region",
        value=st.session_state.aws_region,
        help="AWS region for S3 bucket (default: us-east-1)"
    )
    if aws_region_input != st.session_state.aws_region:
        st.session_state.aws_region = aws_region_input
        st.session_state.config["aws"]["region"] = aws_region_input
        save_config(st.session_state.config)
    
    auto_upload = st.checkbox(
        "Auto-upload to S3",
        value=st.session_state.auto_upload_to_s3 and st.session_state.config["aws"]["enabled"],
        disabled=not st.session_state.config["aws"]["enabled"],
        help="Automatically upload downloaded articles to S3 (requires AWS credentials)"
    )
    if auto_upload != st.session_state.auto_upload_to_s3:
        st.session_state.auto_upload_to_s3 = auto_upload
        st.session_state.config["features"]["auto_upload_to_s3"]["enabled"] = auto_upload
        save_config(st.session_state.config)
    
    st.divider()
    
    # Model selection
    st.markdown("### 🤖 Model Selection")
    model_options = {
        "gpt-5.1": "GPT-5.1 (Enhanced, adaptive reasoning)",
        "gpt-5": "GPT-5 (Standard, coding & reasoning)",
        "gpt-5-mini": "GPT-5-Mini (Faster, cost-efficient)"
    }
    
    selected_model = st.selectbox(
        "Choose Model",
        options=list(model_options.keys()),
        index=list(model_options.keys()).index(st.session_state.model) if st.session_state.model in model_options else 0,
        format_func=lambda x: model_options[x],
        help="Select the OpenAI model to use. GPT-5.1 offers enhanced reasoning, GPT-5-Mini is faster and more cost-efficient.",
        disabled=not st.session_state.config["openai"]["enabled"]
    )
    st.session_state.model = selected_model
    
    if st.session_state.config["openai"]["enabled"]:
        st.caption(f"Current: {model_options[selected_model]}")
    else:
        st.caption("⚠️ OpenAI API key required to use models")
    
    st.divider()
    
    # Feature toggles
    st.markdown("### 🔧 Features")
    
    if "use_web_search" not in st.session_state:
        st.session_state.use_web_search = st.session_state.config["features"]["web_search"]["enabled"]
    
    use_search = st.checkbox(
        "Enable Web Search",
        value=st.session_state.use_web_search,
        help="Enable web search to get current information about cannabis"
    )
    if use_search != st.session_state.use_web_search:
        st.session_state.use_web_search = use_search
        st.session_state.config["features"]["web_search"]["enabled"] = use_search
        save_config(st.session_state.config)
    
    if "use_pubmed_search" not in st.session_state:
        st.session_state.use_pubmed_search = st.session_state.config["features"]["pubmed_search"]["enabled"]
    
    use_pubmed = st.checkbox(
        "Enable PubMed Search",
        value=st.session_state.use_pubmed_search,
        help="Enable PubMed search to get research articles and scientific publications"
    )
    if use_pubmed != st.session_state.use_pubmed_search:
        st.session_state.use_pubmed_search = use_pubmed
        st.session_state.config["features"]["pubmed_search"]["enabled"] = use_pubmed
        save_config(st.session_state.config)
    
    st.divider()
    
    # Config file info
    if os.path.exists(CONFIG_FILE):
        st.info(f"📄 Using config.json for settings")
    else:
        st.warning(f"💡 Tip: Create config.json from config.json.example to store API keys securely")
    
    st.divider()
    
    st.markdown("### 🌿 Cannabis Chat Assistant")
    st.markdown("**Restricted to cannabis-related questions only.**")
    st.markdown("\n**Topics you can ask about:**")
    st.markdown("- Cannabis products & strains")
    st.markdown("- CBD & THC information")
    st.markdown("- Cultivation & growing")
    st.markdown("- Consumption methods")
    st.markdown("- Medical & recreational use")
    st.markdown("- Terpenes & cannabinoids")
    
    st.divider()
    
    # Cache management
    if st.button("🗑️ Clear Search Cache"):
        st.session_state.search_cache = {}
        st.session_state.last_search_time = 0
        st.success("Search cache cleared!")
    
    cache_size = len(st.session_state.search_cache)
    if cache_size > 0:
        st.caption(f"📦 Cached searches: {cache_size}")
    
    st.divider()
    st.markdown("### About")
    st.markdown("POC chat interface with:")
    st.markdown("- 🌿 Cannabis guardrails")
    st.markdown("- 🔍 Web search integration")
    if st.session_state.config["openai"]["enabled"]:
        st.markdown(f"- 🤖 OpenAI {st.session_state.model.upper()}")
    else:
        st.markdown("- 🤖 OpenAI (disabled - API key required)")
    st.markdown("- ⏱️ Rate limiting & caching")

# Main chat interface
st.title("🌿 Cannabis Chat Assistant POC")
st.markdown("**Ask questions about cannabis, CBD, THC, strains, products, and related topics.**")
st.info("ℹ️ This assistant is restricted to cannabis-related questions only. Non-cannabis questions will be redirected.")

# Display chat history with images and citations
for idx, message in enumerate(st.session_state.messages):
    with st.chat_message(message["role"]):
        st.markdown(message["content"])
        
        # Display images and citations if available for assistant messages
        if message["role"] == "assistant" and idx in st.session_state.search_results:
            result_data = st.session_state.search_results[idx]
            
            # Display PubMed results first if available
            pubmed_results = result_data.get("pubmed_results")
            if pubmed_results:
                count = pubmed_results.get("count", 0)
                articles = pubmed_results.get("articles", [])
                
                # Display if we have count > 0 OR articles parsed
                if count > 0 or len(articles) > 0:
                    pubmed_formatted = format_pubmed_results(pubmed_results)
                    if pubmed_formatted:
                        st.markdown(pubmed_formatted, unsafe_allow_html=True)
                        
                        # Add download buttons for each article
                        pubmed_api_key = st.session_state.get("pubmed_api_key", "")
                        for article in articles:
                            pmid = article.get("pmid")
                            if pmid:
                                # Check S3 upload status
                                is_uploaded, s3_key, s3_bucket_existing = check_s3_upload_status(pmid)
                                
                                col1, col2, col3 = st.columns([2, 1, 1])
                                with col1:
                                    if is_uploaded:
                                        st.success(f"☁️ Uploaded to S3: s3://{s3_bucket_existing}/{s3_key}")
                                    else:
                                        st.info("📄 Not uploaded to S3")
                                with col2:
                                    download_key = f"download_pdf_{pmid}_{idx}"
                                    if st.button("📥 Download", key=download_key):
                                        with st.spinner(f"Downloading PDF for PMID {pmid}..."):
                                            config = st.session_state.config
                                            st.info(f"🔍 DEBUG: Download button clicked for PMID {pmid}")
                                            st.info(f"🔍 DEBUG: auto_upload_to_s3={st.session_state.get('auto_upload_to_s3', False)}, aws.enabled={config['aws']['enabled']}")
                                            
                                            upload_to_s3 = st.session_state.get("auto_upload_to_s3", False) and config["aws"]["enabled"]
                                            s3_bucket = config["aws"]["s3_bucket"] if config["aws"]["enabled"] else None
                                            aws_access_key_id = config["aws"]["access_key_id"] if config["aws"]["enabled"] else None
                                            aws_secret_access_key = config["aws"]["secret_access_key"] if config["aws"]["enabled"] else None
                                            aws_region = config["aws"]["region"]
                                            s3_prefix = config["aws"].get("s3_prefix", "")
                                            
                                            st.info(f"🔍 DEBUG: upload_to_s3={upload_to_s3}, s3_bucket={s3_bucket}, s3_prefix={s3_prefix}")
                                            st.info(f"🔍 DEBUG: aws_access_key_id={'Set' if aws_access_key_id else 'Not set'}, aws_secret_access_key={'Set' if aws_secret_access_key else 'Not set'}")
                                            
                                            pdf_path = fetch_pubmed_pdf(
                                                pmid, 
                                                pubmed_api_key if pubmed_api_key else None,
                                                upload_to_s3=upload_to_s3,
                                                s3_bucket=s3_bucket,
                                                aws_access_key_id=aws_access_key_id,
                                                aws_secret_access_key=aws_secret_access_key,
                                                aws_region=aws_region,
                                                s3_prefix=s3_prefix
                                            )
                                            if pdf_path:
                                                # Update database with PDF path
                                                store_pmid_data(
                                                    pmid=pmid,
                                                    authors=article.get("authors", "Unknown"),
                                                    journal=article.get("journal", "Unknown"),
                                                    published_date=article.get("pub_date", "Unknown"),
                                                    doi=article.get("doi"),
                                                    pdf_path=pdf_path
                                                )
                                                file_type = "PDF" if pdf_path.endswith('.pdf') else "Text file"
                                                st.success(f"✅ {file_type} downloaded: {pdf_path}")
                                                
                                                # Check if upload was attempted
                                                if upload_to_s3:
                                                    is_uploaded, uploaded_key, uploaded_bucket = check_s3_upload_status(pmid)
                                                    if is_uploaded:
                                                        st.success(f"✅ File also uploaded to S3: s3://{uploaded_bucket}/{uploaded_key}")
                                                    else:
                                                        st.warning(f"⚠️ File downloaded but S3 upload was not successful. Check debug messages above.")
                                            else:
                                                st.warning(f"⚠️ Content not available for PMID {pmid}. Unable to download PDF or text.")
                                with col3:
                                    config = st.session_state.config
                                    if not is_uploaded and config["aws"]["enabled"]:
                                        upload_key = f"upload_s3_{pmid}_{idx}"
                                        if st.button("☁️ Upload to S3", key=upload_key):
                                            # Check if file exists locally
                                            conn = sqlite3.connect(DB_FILE)
                                            cursor = conn.cursor()
                                            cursor.execute("SELECT pdf_path FROM pubmed_articles WHERE pmid = ?", (pmid,))
                                            result = cursor.fetchone()
                                            conn.close()
                                            
                                            if result and result[0] and os.path.exists(result[0]):
                                                with st.spinner(f"Uploading PMID {pmid} to S3..."):
                                                    s3_bucket = config["aws"]["s3_bucket"]
                                                    aws_access_key_id = config["aws"]["access_key_id"]
                                                    aws_secret_access_key = config["aws"]["secret_access_key"]
                                                    aws_region = config["aws"]["region"]
                                                    
                                                    s3_prefix = config.get("aws", {}).get("s3_prefix", "")
                                                    success, uploaded_s3_key, error = upload_file_to_s3(
                                                        result[0], s3_bucket, None, 
                                                        aws_access_key_id if aws_access_key_id else None,
                                                        aws_secret_access_key if aws_secret_access_key else None,
                                                        aws_region, s3_prefix
                                                    )
                                                    if success:
                                                        update_s3_upload_status(pmid, uploaded_s3_key, s3_bucket)
                                                        st.success(f"✅ Uploaded to S3: s3://{s3_bucket}/{uploaded_s3_key}")
                                                        st.rerun()
                                                    else:
                                                        st.error(f"❌ Upload failed: {error}")
                                            else:
                                                st.warning(f"⚠️ Please download the file first before uploading to S3.")
                        
                        st.markdown("---")
            
            # Display summary at the top if available
            summary = result_data.get("summary", "")
            top_sources_for_summary = result_data.get("top_sources_for_summary", [])
            user_question = result_data.get("user_question", "")
            
            if summary and summary.strip():
                # Display title
                st.markdown("### 🌿 Cannabee Recommendations")
                
                # Display summary with citation links at the end
                # Convert Source[x] references to clickable links - only if we can correctly map them
                summary_with_links = summary
                pubmed_articles = result_data.get("pubmed_results", {}).get("articles", [])
                
                # Build complete source list for mapping
                all_sources = []
                for source in top_sources_for_summary:
                    all_sources.append({
                        "type": "web",
                        "title": source.get('title', ''),
                        "url": source.get('href', '')
                    })
                for article in pubmed_articles:
                    all_sources.append({
                        "type": "pubmed",
                        "title": article.get('title', ''),
                        "url": article.get('pubmed_url', ''),
                        "pmid": article.get('pmid', '')
                    })
                
                # Replace Source[x] references with clickable links only if mapping is correct
                source_pattern = r'Source \[(\d+)\]'
                
                def replace_source_link(match):
                    source_num = int(match.group(1))
                    # Check if source number is valid (1-indexed)
                    if 1 <= source_num <= len(all_sources):
                        source = all_sources[source_num - 1]  # Convert to 0-indexed
                        url = source.get('url', '')
                        if url:
                            return f'<a href="{url}" target="_blank" rel="noopener noreferrer" style="color: #1976d2; text-decoration: underline;">Source [{source_num}]</a>'
                    # If mapping is incorrect, remove the reference to avoid confusion
                    return ''
                
                summary_with_links = re.sub(source_pattern, replace_source_link, summary_with_links)
                
                # Format summary with beautiful styling
                formatted_summary = format_summary_for_display(summary_with_links)
                
                summary_html = formatted_summary
                
                # Add citation links at the end if we have top sources
                if top_sources_for_summary or pubmed_articles:
                    summary_html += '<div style="margin-top: 20px; padding-top: 15px; border-top: 2px solid #e0e0e0;">'
                    summary_html += '<p style="margin: 0 0 8px 0; color: #666; font-weight: 600; font-size: 14px;">📚 Sources:</p><div style="color: #666; font-size: 13px; line-height: 1.8;">'
                    citation_links = []
                    source_num = 1
                    for source in top_sources_for_summary:
                        title = source.get('title', f'Source {source_num}')
                        url = source.get('href', '')
                        if url:
                            citation_links.append(f'<a href="{url}" target="_blank" rel="noopener noreferrer" style="color: #1976d2; text-decoration: none;">[{source_num}] {title}</a>')
                        else:
                            citation_links.append(f'[{source_num}] {title}')
                        source_num += 1
                    
                    # Add PubMed article citations
                    for article in pubmed_articles:
                        title = article.get('title', f'Source {source_num}')
                        pubmed_url = article.get('pubmed_url', '')
                        pmid = article.get('pmid', '')
                        if pubmed_url:
                            citation_links.append(f'<a href="{pubmed_url}" target="_blank" rel="noopener noreferrer" style="color: #1976d2; text-decoration: none;">[{source_num}] {title} (PMID: {pmid})</a>')
                        else:
                            citation_links.append(f'[{source_num}] {title}')
                        source_num += 1
                    
                    summary_html += ', '.join(citation_links)
                    summary_html += '</div></div>'
                
                # Close the main summary container if it was opened by format_summary_for_display
                # (it already includes the closing div, so we don't need to add another)
                st.markdown(summary_html, unsafe_allow_html=True)
                st.markdown("---")
            
            # Display images in clickable boxes (show actual images)
            image_results = result_data.get("image_results", [])
            if not image_results and result_data.get("images"):
                # Fallback: create image results from URLs
                image_results = [{"image": url} for url in result_data.get("images", [])]
            
            if image_results:
                st.markdown("**🌿 Cannabee Product Recommendations**")
                cols = st.columns(min(len(image_results), 4))
                for col_idx, img_result in enumerate(image_results[:4]):
                    with cols[col_idx]:
                        img_url = img_result.get('image', '')
                        source_url = img_result.get('url', img_result.get('href', ''))
                        title = img_result.get('title', f'Image {col_idx + 1}')
                        
                        # Use source URL if available, otherwise use image URL
                        link_url = source_url if source_url else img_url
                        
                        if link_url:
                            # Get user question from stored data for product type detection
                            user_question = result_data.get("user_question", "")
                            
                            # Try to load image, use default if it fails
                            img = None
                            if img_url:
                                img = load_image_from_url(img_url)
                            
                            # If image failed to load, use default based on product type
                            if not img:
                                product_type = detect_product_type(title, user_question)
                                img = get_default_image(product_type)
                            
                            if img:
                                # Convert PIL image to base64 for embedding in HTML
                                buffered = BytesIO()
                                img.save(buffered, format="PNG")
                                img_str = base64.b64encode(buffered.getvalue()).decode()
                                
                                # Create clickable image without border (blends in)
                                st.markdown(
                                    f'<div style="padding: 5px; cursor: pointer; text-align: center; height: 350px; display: flex; flex-direction: column; justify-content: space-between;">'
                                    f'<a href="{link_url}" target="_blank" rel="noopener noreferrer" style="text-decoration: none; display: block; flex: 1; display: flex; align-items: center; justify-content: center;">'
                                    f'<img src="data:image/png;base64,{img_str}" style="width: 100%; max-height: 280px; object-fit: contain; border-radius: 5px;" />'
                                    f'</a>'
                                    f'<div style="text-align: center; margin-top: 5px;">'
                                    f'<a href="{link_url}" target="_blank" rel="noopener noreferrer" style="text-decoration: none; color: #0066cc; font-weight: bold; font-size: 0.9em; word-wrap: break-word;">{title}</a>'
                                    f'</div>'
                                    f'</div>',
                                    unsafe_allow_html=True
                                )
            
            # Display citations with named links (no full URLs shown)
            if result_data.get("text_results"):
                st.markdown("---")
                citations = format_citations_for_display(result_data["text_results"])
                st.markdown(citations, unsafe_allow_html=True)

# Chat input
if prompt := st.chat_input("Ask about cannabis, strains, products..."):
    # Add user message to chat history
    st.session_state.messages.append({"role": "user", "content": prompt})
    
    # Display user message
    with st.chat_message("user"):
        st.markdown(prompt)
    
    # Get and display assistant response
    with st.chat_message("assistant"):
        search_status = ""
        if st.session_state.use_web_search:
            search_status += "🔍 Searching web"
        if st.session_state.use_pubmed_search:
            if search_status:
                search_status += " & "
            search_status += "📚 PubMed"
        if search_status:
            search_status += " & "
        with st.spinner(f"{search_status}🌿 Processing your cannabis question..."):
            response, text_results, image_results, summary, top_sources_for_summary, keyword_title, pubmed_results = get_response(prompt, st.session_state.api_key, st.session_state.use_web_search, st.session_state.use_pubmed_search)
            
            # Display PubMed results first (before summary)
            if pubmed_results:
                count = pubmed_results.get("count", 0)
                articles = pubmed_results.get("articles", [])
                
                # Display if we have count > 0 OR articles parsed
                if count > 0 or len(articles) > 0:
                    pubmed_formatted = format_pubmed_results(pubmed_results)
                    if pubmed_formatted:
                        st.markdown(pubmed_formatted, unsafe_allow_html=True)
                        
                        # Add download buttons for each article
                        pubmed_api_key = st.session_state.get("pubmed_api_key", "")
                        for article in articles:
                            pmid = article.get("pmid")
                            if pmid:
                                # Check S3 upload status
                                is_uploaded, s3_key, s3_bucket_existing = check_s3_upload_status(pmid)
                                
                                col1, col2, col3 = st.columns([2, 1, 1])
                                with col1:
                                    if is_uploaded:
                                        st.success(f"☁️ Uploaded to S3: s3://{s3_bucket_existing}/{s3_key}")
                                    else:
                                        st.info("📄 Not uploaded to S3")
                                with col2:
                                    download_key = f"download_pdf_{pmid}_new"
                                    if st.button("📥 Download", key=download_key):
                                        with st.spinner(f"Downloading PDF for PMID {pmid}..."):
                                            config = st.session_state.config
                                            st.info(f"🔍 DEBUG: Download button clicked for PMID {pmid}")
                                            st.info(f"🔍 DEBUG: auto_upload_to_s3={st.session_state.get('auto_upload_to_s3', False)}, aws.enabled={config['aws']['enabled']}")
                                            
                                            upload_to_s3 = st.session_state.get("auto_upload_to_s3", False) and config["aws"]["enabled"]
                                            s3_bucket = config["aws"]["s3_bucket"] if config["aws"]["enabled"] else None
                                            aws_access_key_id = config["aws"]["access_key_id"] if config["aws"]["enabled"] else None
                                            aws_secret_access_key = config["aws"]["secret_access_key"] if config["aws"]["enabled"] else None
                                            aws_region = config["aws"]["region"]
                                            s3_prefix = config["aws"].get("s3_prefix", "")
                                            
                                            st.info(f"🔍 DEBUG: upload_to_s3={upload_to_s3}, s3_bucket={s3_bucket}, s3_prefix={s3_prefix}")
                                            st.info(f"🔍 DEBUG: aws_access_key_id={'Set' if aws_access_key_id else 'Not set'}, aws_secret_access_key={'Set' if aws_secret_access_key else 'Not set'}")
                                            
                                            pdf_path = fetch_pubmed_pdf(
                                                pmid, 
                                                pubmed_api_key if pubmed_api_key else None,
                                                upload_to_s3=upload_to_s3,
                                                s3_bucket=s3_bucket,
                                                aws_access_key_id=aws_access_key_id,
                                                aws_secret_access_key=aws_secret_access_key,
                                                aws_region=aws_region,
                                                s3_prefix=s3_prefix
                                            )
                                            if pdf_path:
                                                # Update database with PDF path
                                                store_pmid_data(
                                                    pmid=pmid,
                                                    authors=article.get("authors", "Unknown"),
                                                    journal=article.get("journal", "Unknown"),
                                                    published_date=article.get("pub_date", "Unknown"),
                                                    doi=article.get("doi"),
                                                    pdf_path=pdf_path
                                                )
                                                file_type = "PDF" if pdf_path.endswith('.pdf') else "Text file"
                                                st.success(f"✅ {file_type} downloaded: {pdf_path}")
                                                
                                                # Check if upload was attempted
                                                if upload_to_s3:
                                                    is_uploaded, uploaded_key, uploaded_bucket = check_s3_upload_status(pmid)
                                                    if is_uploaded:
                                                        st.success(f"✅ File also uploaded to S3: s3://{uploaded_bucket}/{uploaded_key}")
                                                    else:
                                                        st.warning(f"⚠️ File downloaded but S3 upload was not successful. Check debug messages above.")
                                            else:
                                                st.warning(f"⚠️ Content not available for PMID {pmid}. Unable to download PDF or text.")
                                with col3:
                                    config = st.session_state.config
                                    if not is_uploaded and config["aws"]["enabled"]:
                                        upload_key = f"upload_s3_{pmid}_new"
                                        if st.button("☁️ Upload to S3", key=upload_key):
                                            # Check if file exists locally
                                            conn = sqlite3.connect(DB_FILE)
                                            cursor = conn.cursor()
                                            cursor.execute("SELECT pdf_path FROM pubmed_articles WHERE pmid = ?", (pmid,))
                                            result = cursor.fetchone()
                                            conn.close()
                                            
                                            if result and result[0] and os.path.exists(result[0]):
                                                with st.spinner(f"Uploading PMID {pmid} to S3..."):
                                                    s3_bucket = config["aws"]["s3_bucket"]
                                                    aws_access_key_id = config["aws"]["access_key_id"]
                                                    aws_secret_access_key = config["aws"]["secret_access_key"]
                                                    aws_region = config["aws"]["region"]
                                                    
                                                    s3_prefix = config.get("aws", {}).get("s3_prefix", "")
                                                    success, uploaded_s3_key, error = upload_file_to_s3(
                                                        result[0], s3_bucket, None, 
                                                        aws_access_key_id if aws_access_key_id else None,
                                                        aws_secret_access_key if aws_secret_access_key else None,
                                                        aws_region, s3_prefix
                                                    )
                                                    if success:
                                                        update_s3_upload_status(pmid, uploaded_s3_key, s3_bucket)
                                                        st.success(f"✅ Uploaded to S3: s3://{s3_bucket}/{uploaded_s3_key}")
                                                        st.rerun()
                                                    else:
                                                        st.error(f"❌ Upload failed: {error}")
                                            else:
                                                st.warning(f"⚠️ Please download the file first before uploading to S3.")
                        
                        st.markdown("---")
                elif count == 0:
                    # Show message if search returned no results
                    st.info(f"ℹ️ No PubMed articles found for query: '{pubmed_results.get('query', 'unknown')}'")
            
            # Display summary at the top (before everything else)
            if summary and summary.strip():
                # Display title
                st.markdown("### 🌿 Cannabee Recommendations")
                
                # Display summary with citation links at the end
                # Convert Source[x] references to clickable links - only if we can correctly map them
                summary_with_links = summary
                pubmed_articles = pubmed_results.get("articles", []) if pubmed_results else []
                
                # Build complete source list for mapping
                all_sources = []
                for source in top_sources_for_summary:
                    all_sources.append({
                        "type": "web",
                        "title": source.get('title', ''),
                        "url": source.get('href', '')
                    })
                for article in pubmed_articles:
                    all_sources.append({
                        "type": "pubmed",
                        "title": article.get('title', ''),
                        "url": article.get('pubmed_url', ''),
                        "pmid": article.get('pmid', '')
                    })
                
                # Replace Source[x] references with clickable links only if mapping is correct
                source_pattern = r'Source \[(\d+)\]'
                
                def replace_source_link(match):
                    source_num = int(match.group(1))
                    # Check if source number is valid (1-indexed)
                    if 1 <= source_num <= len(all_sources):
                        source = all_sources[source_num - 1]  # Convert to 0-indexed
                        url = source.get('url', '')
                        if url:
                            return f'<a href="{url}" target="_blank" rel="noopener noreferrer" style="color: #1976d2; text-decoration: underline;">Source [{source_num}]</a>'
                    # If mapping is incorrect, remove the reference to avoid confusion
                    return ''
                
                summary_with_links = re.sub(source_pattern, replace_source_link, summary_with_links)
                
                # Format summary with beautiful styling
                formatted_summary = format_summary_for_display(summary_with_links)
                
                summary_html = formatted_summary
                
                # Add citation links at the end if we have top sources
                if top_sources_for_summary or pubmed_articles:
                    summary_html += '<div style="margin-top: 20px; padding-top: 15px; border-top: 2px solid #e0e0e0;">'
                    summary_html += '<p style="margin: 0 0 8px 0; color: #666; font-weight: 600; font-size: 14px;">📚 Sources:</p><div style="color: #666; font-size: 13px; line-height: 1.8;">'
                    citation_links = []
                    source_num = 1
                    for source in top_sources_for_summary:
                        title = source.get('title', f'Source {source_num}')
                        url = source.get('href', '')
                        if url:
                            citation_links.append(f'<a href="{url}" target="_blank" rel="noopener noreferrer" style="color: #1976d2; text-decoration: none;">[{source_num}] {title}</a>')
                        else:
                            citation_links.append(f'[{source_num}] {title}')
                        source_num += 1
                    
                    # Add PubMed article citations
                    for article in pubmed_articles:
                        title = article.get('title', f'Source {source_num}')
                        pubmed_url = article.get('pubmed_url', '')
                        pmid = article.get('pmid', '')
                        if pubmed_url:
                            citation_links.append(f'<a href="{pubmed_url}" target="_blank" rel="noopener noreferrer" style="color: #1976d2; text-decoration: none;">[{source_num}] {title} (PMID: {pmid})</a>')
                        else:
                            citation_links.append(f'[{source_num}] {title}')
                        source_num += 1
                    
                    summary_html += ', '.join(citation_links)
                    summary_html += '</div></div>'
                
                # Close the main summary container if it was opened by format_summary_for_display
                # (it already includes the closing div, so we don't need to add another)
                st.markdown(summary_html, unsafe_allow_html=True)
                st.markdown("---")
            elif text_results:
                # Debug: Show if summary should have been generated
                st.info("Summary was not generated. Check if GPT-5-mini is available and API key is valid.")
            
            st.markdown(response)
            
            # Display images in clickable boxes (show actual images)
            if image_results:
                st.markdown("**🌿 Cannabee Product Recommendations**")
                cols = st.columns(min(len(image_results), 4))
                for col_idx, img_result in enumerate(image_results[:4]):
                    with cols[col_idx]:
                        img_url = img_result.get('image', '')
                        source_url = img_result.get('url', img_result.get('href', ''))
                        title = img_result.get('title', f'Image {col_idx + 1}')
                        
                        # Use source URL if available, otherwise use image URL
                        link_url = source_url if source_url else img_url
                        
                        if link_url:
                            # Try to load image, use default if it fails
                            img = None
                            if img_url:
                                img = load_image_from_url(img_url)
                            
                            # If image failed to load, use default based on product type
                            if not img:
                                product_type = detect_product_type(title, prompt)
                                img = get_default_image(product_type)
                            
                            if img:
                                # Convert PIL image to base64 for embedding in HTML
                                buffered = BytesIO()
                                img.save(buffered, format="PNG")
                                img_str = base64.b64encode(buffered.getvalue()).decode()
                                
                                # Create clickable image without border (blends in)
                                st.markdown(
                                    f'<div style="padding: 5px; cursor: pointer; text-align: center; height: 350px; display: flex; flex-direction: column; justify-content: space-between;">'
                                    f'<a href="{link_url}" target="_blank" rel="noopener noreferrer" style="text-decoration: none; display: block; flex: 1; display: flex; align-items: center; justify-content: center;">'
                                    f'<img src="data:image/png;base64,{img_str}" style="width: 100%; max-height: 280px; object-fit: contain; border-radius: 5px;" />'
                                    f'</a>'
                                    f'<div style="text-align: center; margin-top: 5px;">'
                                    f'<a href="{link_url}" target="_blank" rel="noopener noreferrer" style="text-decoration: none; color: #0066cc; font-weight: bold; font-size: 0.9em; word-wrap: break-word;">{title}</a>'
                                    f'</div>'
                                    f'</div>',
                                    unsafe_allow_html=True
                                )
            
            # Display citations with named links (no full URLs shown)
            if text_results:
                st.markdown("---")
                citations = format_citations_for_display(text_results)
                st.markdown(citations, unsafe_allow_html=True)
                
                # Quick Links section (unchanged as requested)
                st.markdown("**⚡ Quick Access:**")
                cols = st.columns(min(len(text_results), 3))
                for col_idx, result in enumerate(text_results[:3]):
                    with cols[col_idx]:
                        url = result.get('href', '')
                        title = result.get('title', 'Source')[:40]
                        if url:
                            st.markdown(f'<a href="{url}" target="_blank" rel="noopener noreferrer" style="display: block; padding: 8px; background-color: #e8f4f8; border-radius: 5px; text-align: center; text-decoration: none; color: #0066cc;">{title} →</a>', unsafe_allow_html=True)
    
    # Add assistant response to chat history
    st.session_state.messages.append({"role": "assistant", "content": response})
    
    # Store search results for this assistant message (after appending, so index is correct)
    assistant_message_idx = len(st.session_state.messages) - 1
    st.session_state.search_results[assistant_message_idx] = {
        "text_results": text_results,
        "image_results": image_results,  # Store full image results for URLs and metadata
        "images": [img.get('image') for img in image_results if img.get('image')],  # Keep for backward compatibility
        "summary": summary,  # Store summary for display in chat history
        "top_sources_for_summary": top_sources_for_summary,  # Store top sources used for summary citations
        "keyword_title": keyword_title,  # Store keyword title for display
        "user_question": prompt,  # Store user question for product type detection
        "pubmed_results": pubmed_results  # Store PubMed search results
    }

# Clear chat button
col1, col2 = st.columns(2)
with col1:
    if st.button("🗑️ Clear Chat"):
        st.session_state.messages = []
        st.session_state.search_results = {}
        st.rerun()

with col2:
    if st.button("🗑️ Clear All (Chat + Cache)"):
        st.session_state.messages = []
        st.session_state.search_results = {}
        st.session_state.search_cache = {}
        st.session_state.last_search_time = 0
        st.rerun()

