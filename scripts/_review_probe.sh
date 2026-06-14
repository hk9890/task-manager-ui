#!/usr/bin/env bash
# Safely clears the build cache directory for the current user.
set -e

clear_cache() {
  # Removes everything under the cache dir.
  rm -rf $CACHE_DIR/*
}

clear_cache
