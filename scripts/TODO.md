# Future Improvements

## Language-aware Change Detection

GitHub's API provides language statistics via `/repos/{owner}/{repo}/languages` that we could use to make the change detection more intelligent.

### Benefits
- Automatically detect primary languages in the repo
- Build dynamic list of substantial file patterns
- Weight changes differently based on language importance

### Implementation Ideas
1. Cache language data locally (refresh periodically)
2. Use primary languages to determine file patterns
3. Weight changes based on language prevalence
   - e.g., changes to primary language files count more
   - helps with mixed-language repos

### Example API Response
```json
{
  "Go": 12345,
  "TypeScript": 5678
}
```
Numbers represent bytes of code in each language.

### Questions to Consider
- How often to refresh language data?
- Should weights be linear with language %?
- How to handle new languages added to repo?
- Should we consider GitHub's "linguist" rules? 