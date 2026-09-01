# Verifies the agent-readiness of the GitHub Pages site (sanketpatel32.github.io/Blunt-code).
# Checks every public endpoint and machine-readable file published from the gh-pages branch:
# homepage content + metadata + JSON-LD, llms.txt, sitemap.xml, robots.txt, index.md,
# og-image, trust pages (about/privacy/contact), and the custom 404.
#
# Usage:  powershell -File scripts\verify-site.ps1 [-BaseUrl https://sanketpatel32.github.io/Blunt-code]
# Exit code 0 = all checks passed.

param(
    [string]$BaseUrl = "https://sanketpatel32.github.io/Blunt-code"
)

$ErrorActionPreference = "Stop"
$pass = 0
$fail = 0

function Get-Page([string]$Url) {
    try {
        $r = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 30
        return @{ Status = [int]$r.StatusCode; Content = [string]$r.Content; Type = [string]$r.Headers["Content-Type"] }
    } catch {
        $code = 0
        if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        return @{ Status = $code; Content = ""; Type = "" }
    }
}

function Assert([bool]$Condition, [string]$Name) {
    if ($Condition) {
        Write-Output ("PASS  " + $Name)
        $script:pass++
    } else {
        Write-Output ("FAIL  " + $Name)
        $script:fail++
    }
}

# --- Homepage ---------------------------------------------------------------
$homepage = Get-Page $BaseUrl
Assert ($homepage.Status -eq 200) "homepage returns 200"
Assert ($homepage.Content.Length -ge 500) "homepage serves 500+ chars without JS (got $($homepage.Content.Length))"
Assert ($homepage.Content -match "<h1") "homepage has an H1"
Assert ($homepage.Content -match 'lang="en"') "homepage declares <html lang>"
Assert ($homepage.Content -match 'rel="canonical"') "homepage has canonical link"
Assert ($homepage.Content -match 'property="og:image"') "homepage has og:image"
Assert ($homepage.Content -match 'property="og:type"') "homepage has og:type"

# --- JSON-LD ----------------------------------------------------------------
if ($homepage.Content -match '(?s)<script type="application/ld\+json">\s*(\{.*?\})\s*</script>') {
    try {
        $ld = $Matches[1] | ConvertFrom-Json
        Assert ($ld."@type" -eq "SoftwareApplication") "JSON-LD parses as SoftwareApplication"
        Assert ($ld.name -eq "Blunt Code" -and $ld.url -and $ld.description) "JSON-LD has name/url/description"
        Assert ($ld.publisher.contactPoint.url -like "*issues*") "JSON-LD publisher has contactPoint"
    } catch {
        Assert $false "JSON-LD parses as valid JSON ($($_.Exception.Message))"
    }
} else {
    Assert $false "homepage contains JSON-LD block"
}

# --- Machine-readable files ---------------------------------------------------
$llms = Get-Page "$BaseUrl/llms.txt"
Assert ($llms.Status -eq 200) "llms.txt returns 200"
Assert ($llms.Content.StartsWith("# Blunt Code")) "llms.txt follows llmstxt.org format (H1 title)"
Assert ($llms.Content -match "## When to use") "llms.txt has when-to-use guidance"

$sitemap = Get-Page "$BaseUrl/sitemap.xml"
Assert ($sitemap.Status -eq 200) "sitemap.xml returns 200"
Assert ($sitemap.Content -match "sitemaps.org/schemas") "sitemap.xml is a valid sitemap document"
Assert (($sitemap.Content | Select-String -Pattern "<loc>" -AllMatches).Matches.Count -ge 4) "sitemap.xml lists 4+ URLs"

$robots = Get-Page "$BaseUrl/robots.txt"
Assert ($robots.Status -eq 200) "robots.txt returns 200"
Assert ($robots.Content -match "User-agent: ClaudeBot" -and $robots.Content -match "User-agent: GPTBot") "robots.txt explicitly allows AI crawlers"

$md = Get-Page "$BaseUrl/index.md"
Assert ($md.Status -eq 200 -and $md.Content.Length -ge 500) "index.md (markdown edition) returns 200 with content"

$og = Get-Page "$BaseUrl/og-image.png"
Assert ($og.Status -eq 200) "og-image.png returns 200"
Assert ($og.Type -like "image/png*") "og-image.png is image/png"

# --- Trust anchor pages --------------------------------------------------------
foreach ($p in @("about", "privacy", "contact")) {
    $page = Get-Page "$BaseUrl/$p/"
    Assert ($page.Status -eq 200) "/$p/ returns 200"
    Assert ($page.Content.Length -ge 500) "/$p/ has 500+ chars of content (got $($page.Content.Length))"
    Assert ($page.Content -match 'rel="canonical"') "/$p/ has canonical link"
}

# --- Custom 404 -----------------------------------------------------------------
$nf = Get-Page "$BaseUrl/path-that-does-not-exist-xyz"
Assert ($nf.Status -eq 404) "nonexistent path returns real 404 (got $($nf.Status))"
if ($nf.Status -eq 404) {
    $nf404 = Get-Page "$BaseUrl/404.html"
    $body = if ($nf404.Status -eq 200) { $nf404.Content } else { "" }
    Assert ($body -match "sitemap\.xml" -and $body -match "llms\.txt") "404 page points agents at sitemap and llms.txt"
}

Write-Output ""
Write-Output ("{0} passed, {1} failed" -f $pass, $fail)
if ($fail -gt 0) { exit 1 } else { exit 0 }
