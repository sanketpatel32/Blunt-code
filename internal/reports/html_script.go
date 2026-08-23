package reports

// htmlScript is the report's entire client-side layer, embedded verbatim into
// every export inside the single <script> element rendered by htmlDocument.
//
// Security contract: the script never constructs HTML. Every piece of finding
// metadata it touches (severity class, analyzer id, rule id, file path) was
// HTML-escaped into data-* attributes by the Go renderer; here it is read back
// as inert DOM strings and used only for comparison. All UI updates go through
// hidden, classList, setAttribute, and textContent with numeric or fixed
// strings, so untrusted analyzer output can never become markup at run time.
// The script makes no network requests, persists no state, and derives no
// random identifiers, keeping renders byte-stable.
const htmlScript = `(function () {
"use strict";
var rows = [].slice.call(document.querySelectorAll("tr[data-sev]"));
var groups = [].slice.call(document.querySelectorAll("details.file-group"));
var chips = [].slice.call(document.querySelectorAll("button.chip[data-sev]"));
var analyzerSel = document.getElementById("filter-analyzer");
var searchBox = document.getElementById("filter-search");
var clearBtn = document.getElementById("filter-clear");
var statusEl = document.getElementById("filter-status");
var noMatch = document.getElementById("filter-no-match");
if (!rows.length || !groups.length || !chips.length || !analyzerSel || !searchBox || !clearBtn || !statusEl || !noMatch) {
    return;
}
var timer = null;

function apply() {
    var active = {};
    chips.forEach(function (chip) {
        active[chip.getAttribute("data-sev")] = chip.getAttribute("aria-pressed") === "true";
    });
    var selected = analyzerSel.value;
    var query = searchBox.value.trim().toLowerCase();
    var visible = 0;
    var counts = {};
    rows.forEach(function (row) {
        var sev = row.getAttribute("data-sev");
        var show = active[sev] === true;
        if (show && selected !== "" && row.getAttribute("data-an") !== selected) {
            show = false;
        }
        if (show && query !== "" && row.textContent.toLowerCase().indexOf(query) < 0) {
            show = false;
        }
        row.hidden = !show;
        if (show) {
            visible = visible + 1;
            counts[sev] = (counts[sev] || 0) + 1;
        }
    });
    chips.forEach(function (chip) {
        chip.querySelector(".chip-count").textContent = String(counts[chip.getAttribute("data-sev")] || 0);
    });
    groups.forEach(function (group) {
        group.hidden = group.querySelector("tr[data-sev]:not([hidden])") === null;
    });
    statusEl.textContent = "Showing " + visible + " of " + rows.length + " findings";
    noMatch.hidden = visible !== 0;
}

chips.forEach(function (chip) {
    chip.addEventListener("click", function () {
        var on = chip.getAttribute("aria-pressed") === "true";
        chip.setAttribute("aria-pressed", on ? "false" : "true");
        chip.classList.toggle("is-active", !on);
        apply();
    });
});

analyzerSel.addEventListener("change", apply);

searchBox.addEventListener("input", function () {
    window.clearTimeout(timer);
    timer = window.setTimeout(apply, 150);
});

clearBtn.addEventListener("click", function () {
    chips.forEach(function (chip) {
        chip.setAttribute("aria-pressed", "true");
        chip.classList.add("is-active");
    });
    analyzerSel.value = "";
    searchBox.value = "";
    apply();
});

window.addEventListener("beforeprint", function () {
    document.body.classList.add("is-printing");
    groups.forEach(function (group) {
        group.hidden = false;
        group.open = true;
    });
    rows.forEach(function (row) {
        row.hidden = false;
    });
    noMatch.hidden = true;
});

window.addEventListener("afterprint", function () {
    document.body.classList.remove("is-printing");
    apply();
});
}())`
