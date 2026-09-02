# Verifies the agent-readiness of the GitHub Pages site (sanketpatel32.github.io/Blunt-code)
# or a local checkout directory.
# Checks every public endpoint, bot reachability, machine-readable file,
# markdown alternate links, JSON-LD, sitemap, robots.txt, and agent 404.
#
# Usage:
#   powershell -File scripts\verify-site.ps1 [-BaseUrl https://sanketpatel32.github.io/Blunt-code]
#   powershell -File scripts\verify-site.ps1 -LocalDir ..\bluntcode-gh-pages
#
# Exit code 0 = all checks passed.

param(
    [string]$BaseUrl = "https://sanketpatel32.github.io/Blunt-code",
    [string]$LocalDir = ""
)

$ErrorActionPreference = "Stop"
$pass = 0
$fail = 0

function Assert([bool]$Condition, [string]$Name) {
    if ($Condition) {
        Write-Output ("PASS  " + $Name)
        $script:pass++
    } else {
        Write-Output ("FAIL  " + $Name)
        $script:fail++
    }
}

if ($LocalDir -and (Test-Path $LocalDir)) {
    Write-Output "=== Verifying Local Directory: $LocalDir ==="

    # 1. Check .nojekyll & _headers
    Assert (Test-Path (Join-Path $LocalDir ".nojekyll")) ".nojekyll exists to prevent Jekyll interference"
    Assert (Test-Path (Join-Path $LocalDir "_headers")) "_headers exists for CDN Vary & Content-Type configuration"

    # 2. Check robots.txt
    $robots = Get-Content (Join-Path $LocalDir "robots.txt") -Raw
    Assert ($robots -match "User-agent: ChatGPT-User" -and $robots -match "User-agent: ClaudeBot" -and $robots -match "User-agent: Google-Extended" -and $robots -match "User-agent: DeepSeekBot" -and $robots -match "User-agent: ora-agent") "robots.txt explicitly allows all major AI agent crawlers"
    Assert ($robots -match "Sitemap: https://sanketpatel32.github.io/Blunt-code/sitemap.xml") "robots.txt points to sitemap.xml"

    # 3. Check sitemap.xml
    $sitemap = Get-Content (Join-Path $LocalDir "sitemap.xml") -Raw
    Assert ($sitemap -match "<loc>https://sanketpatel32\.github\.io/Blunt-code/</loc>") "sitemap includes homepage"
    Assert ($sitemap -match "<loc>https://sanketpatel32\.github\.io/Blunt-code/index\.md</loc>") "sitemap includes index.md"
    Assert ($sitemap -match "<loc>https://sanketpatel32\.github\.io/Blunt-code/llms\.txt</loc>") "sitemap includes llms.txt"

    # 4. Check index.html (raw HTML without JS)
    $index = Get-Content (Join-Path $LocalDir "index.html") -Raw
    Assert ($index.Length -ge 500) "index.html serves 500+ chars of raw HTML content (got $($index.Length))"
    Assert ($index -match "<h1") "index.html has clear H1 heading"
    Assert ($index -match 'rel="alternate"\s+type="text/markdown"') "index.html has alternate markdown link"
    Assert ($index -match 'rel="alternate"\s+type="text/plain"') "index.html has alternate llms.txt link"
    Assert ($index -match 'application/ld\+json') "index.html has JSON-LD metadata"

    # 5. Check index.md (Markdown edition)
    $md = Get-Content (Join-Path $LocalDir "index.md") -Raw
    Assert ($md.Length -ge 500) "index.md contains 500+ chars of markdown content (got $($md.Length))"
    Assert ($md -match "^#\s+Blunt Code") "index.md has H1 title"

    # 6. Check llms.txt
    $llms = Get-Content (Join-Path $LocalDir "llms.txt") -Raw
    Assert ($llms -match "^#\s+Blunt Code") "llms.txt adheres to llmstxt.org specification"
    Assert ($llms -match "## When to use") "llms.txt contains agent decision guidance"

    # 7. Check 404.html (Agent-friendly 404 recovery)
    $nf404 = Get-Content (Join-Path $LocalDir "404.html") -Raw
    Assert ($nf404 -match "sitemap\.xml" -and $nf404 -match "llms\.txt" -and $nf404 -match "index\.md") "404.html includes markdown recovery block with sitemap and llms.txt links"

    # 8. Check trust anchor pages
    foreach ($p in @("about", "privacy", "contact")) {
        $pPath = Join-Path $LocalDir "$p\index.html"
        Assert (Test-Path $pPath) "$p/index.html exists"
        $pContent = Get-Content $pPath -Raw
        Assert ($pContent.Length -ge 500) "$p/index.html has 500+ chars of content"
        Assert ($pContent -match 'rel="alternate"\s+type="text/markdown"') "$p/index.html has alternate markdown link"
    }

} else {
    Write-Output "=== Verifying Live Endpoints: $BaseUrl ==="

    function Get-Page([string]$Url, [string]$UserAgent = "Mozilla/5.0", [hashtable]$Headers = @{}) {
        try {
            $r = Invoke-WebRequest -Uri $Url -UserAgent $UserAgent -Headers $Headers -UseBasicParsing -TimeoutSec 30
            return @{ Status = [int]$r.StatusCode; Content = [string]$r.Content; Headers = $r.Headers }
        } catch {
            $code = 0
            if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
            return @{ Status = $code; Content = ""; Headers = @{} }
        }
    }

    # --- Agent Crawler Reachability & Bot Allow Tests ---
    $botAgents = @("ChatGPT-User", "ClaudeBot", "Google-Extended", "DeepSeekBot", "ora-agent")
    foreach ($bot in $botAgents) {
        $botRes = Get-Page "$BaseUrl/" $bot
        Assert ($botRes.Status -eq 200) "AI Crawler reachability: '$bot' receives HTTP 200 on homepage"
        Assert ($botRes.Content.Length -ge 500) "AI Crawler '$bot' receives 500+ chars raw HTML"
    }

    # --- Homepage Content Without JS ---
    $homepage = Get-Page "$BaseUrl/"
    Assert ($homepage.Status -eq 200) "homepage returns 200"
    Assert ($homepage.Content.Length -ge 500) "homepage serves 500+ chars without JS (got $($homepage.Content.Length))"
    Assert ($homepage.Content -match "<h1") "homepage has an H1"
    Assert ($homepage.Content -match 'lang="en"') "homepage declares <html lang>"
    Assert ($homepage.Content -match 'rel="canonical"') "homepage has canonical link"
    Assert ($homepage.Content -match 'rel="alternate"\s+type="text/markdown"') "homepage advertises alternate markdown"

    # --- Machine-readable files ---
    $llms = Get-Page "$BaseUrl/llms.txt"
    Assert ($llms.Status -eq 200) "llms.txt returns 200"
    Assert ($llms.Content.StartsWith("# Blunt Code")) "llms.txt follows llmstxt.org format (H1 title)"

    $md = Get-Page "$BaseUrl/index.md"
    Assert ($md.Status -eq 200 -and $md.Content.Length -ge 500) "index.md returns 200 with content"

    $sitemap = Get-Page "$BaseUrl/sitemap.xml"
    Assert ($sitemap.Status -eq 200) "sitemap.xml returns 200"

    $robots = Get-Page "$BaseUrl/robots.txt"
    Assert ($robots.Status -eq 200) "robots.txt returns 200"
    Assert ($robots.Content -match "User-agent: ClaudeBot" -and $robots.Content -match "User-agent: ChatGPT-User") "robots.txt explicitly allows AI crawlers"

    # --- Custom 404 Recovery ---
    $nf = Get-Page "$BaseUrl/path-that-does-not-exist-xyz"
    Assert ($nf.Status -eq 404) "nonexistent path returns real HTTP 404 (got $($nf.Status))"
    if ($nf.Status -eq 404) {
        $nf404 = Get-Page "$BaseUrl/404.html"
        $body = if ($nf404.Status -eq 200) { $nf404.Content } else { "" }
        Assert ($body -match "sitemap\.xml" -and $body -match "llms\.txt" -and $body -match "index\.md") "404 page points agents at sitemap, llms.txt, and index.md"
    }
}

Write-Output ""
Write-Output ("{0} passed, {1} failed" -f $pass, $fail)
if ($fail -gt 0) { exit 1 } else { exit 0 }
