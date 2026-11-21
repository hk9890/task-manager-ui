# 0008: Store search is the search of the SDK, and taskmgr-ui ranks nothing

The order of a store search is the order the SDK returns, so an issue with the word in its
description can come before one with the word in its title.

Rejected: tokenization or relevance ranking in the adapter, because taskmgr-ui stays a thin adapter
over the search of the SDK: ranking is a capability of the backend, or it does not happen.
