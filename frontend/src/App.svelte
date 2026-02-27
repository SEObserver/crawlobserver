<script>
  import { getSessions, getStats, getPages, getExternalLinks, getInternalLinks, getProgress,
    startCrawl, stopCrawl, resumeCrawl, deleteSession, recomputeDepths, computePageRank, retryFailed,
    subscribeProgress, getTheme, updateTheme,
    getPageHTML, getStorageStats, getPageDetail, getSystemStats,
    getPageRankDistribution, getPageRankTreemap, getPageRankTop,
    getRobotsHosts, getRobotsContent, testRobotsUrls } from './lib/api.js';

  let sessions = $state([]);
  let selectedSession = $state(null);
  let stats = $state(null);
  let pages = $state([]);
  let extLinks = $state([]);
  let intLinks = $state([]);
  let tab = $state('overview');
  let loading = $state(true);
  let error = $state(null);

  // Theme
  let theme = $state({ app_name: 'SEOCrawler', logo_url: '', accent_color: '#7c3aed', mode: 'light' });
  let darkMode = $state(false);

  // Pagination
  const PAGE_SIZE = 100;
  let pagesOffset = $state(0);
  let extLinksOffset = $state(0);
  let intLinksOffset = $state(0);
  let hasMorePages = $state(false);
  let hasMoreExtLinks = $state(false);
  let hasMoreIntLinks = $state(false);

  // Global filters
  let filters = $state({});

  const TAB_FILTERS = {
    overview:     ['url', 'status_code', 'title', 'word_count', 'internal_links_out', 'external_links_out', 'body_size', 'fetch_duration_ms', 'depth', 'pagerank'],
    titles:       ['url', 'title', 'title_length', 'h1'],
    meta:         ['url', 'meta_description', 'meta_desc_length', 'meta_keywords', 'og_title'],
    headings:     ['url', 'h1', 'h2'],
    images:       ['url', 'images_count', 'images_no_alt', 'title', 'word_count'],
    indexability: ['url', 'is_indexable', 'index_reason', 'meta_robots', 'canonical', 'canonical_is_self'],
    response:     ['url', 'status_code', 'content_type', 'content_encoding', 'body_size', 'fetch_duration_ms'],
    internal:     ['source_url', 'target_url', 'anchor_text', 'tag'],
    external:     ['source_url', 'target_url', 'anchor_text', 'rel'],
  };

  // Settings
  let showSettings = $state(false);
  let editTheme = $state({ app_name: '', logo_url: '', accent_color: '#7c3aed', mode: 'light' });
  let savingTheme = $state(false);

  // New crawl form
  let showNewCrawl = $state(false);
  let seedInput = $state('');
  let maxPages = $state(0);
  let maxDepth = $state(0);
  let workers = $state(10);
  let crawlDelay = $state('1s');
  let storeHtml = $state(false);
  let starting = $state(false);

  // Resume modal
  let showResumeModal = $state(false);
  let resumeSessionId = $state(null);
  let resumeMaxPages = $state(0);
  let resumeMaxDepth = $state(0);
  let resumeWorkers = $state(10);
  let resumeDelay = $state('1s');
  let resumeStoreHtml = $state(false);
  let resuming = $state(false);

  // HTML modal
  let showHtmlModal = $state(false);
  let htmlModalData = $state({ url: '', body_html: '' });
  let htmlModalView = $state('render'); // 'render' or 'source'
  let htmlModalLoading = $state(false);

  // Storage stats
  let storageStats = $state(null);

  // System stats (CPU/memory monitoring)
  let systemStats = $state(null);
  let systemStatsInterval = null;

  // Page detail
  let pageDetail = $state(null);
  let pageDetailLoading = $state(false);

  // PageRank tab
  let prSubView = $state('top');
  let prLoading = $state(false);
  let prTopData = $state(null);
  let prTopLimit = $state(50);
  let prTopOffset = $state(0);
  let prDistData = $state(null);
  let prTreemapData = $state(null);
  let prTreemapDepth = $state(2);
  let prTreemapMinPages = $state(1);
  let prTableData = $state(null);
  let prTableOffset = $state(0);
  let prTableDir = $state('');
  let prTooltip = $state(null);

  // Robots.txt tab
  let robotsHosts = $state([]);
  let robotsSelectedHost = $state(null);
  let robotsContent = $state(null);
  let robotsLoading = $state(false);
  let robotsTestUrls = $state('');
  let robotsTestUA = $state('');
  let robotsTestResults = $state(null);
  let robotsTestLoading = $state(false);

  // Live progress
  let liveProgress = $state({});
  let sseConnections = {};

  // --- Theme ---
  async function loadTheme() {
    try {
      const t = await getTheme();
      theme = t;
      darkMode = t.mode === 'dark';
      applyTheme();
    } catch {}
  }

  function applyTheme() {
    document.documentElement.setAttribute('data-theme', darkMode ? 'dark' : 'light');
    if (theme.accent_color) {
      document.documentElement.style.setProperty('--accent', theme.accent_color);
      // Generate a lighter version for backgrounds
      const hex = theme.accent_color.replace('#', '');
      const r = parseInt(hex.substr(0, 2), 16);
      const g = parseInt(hex.substr(2, 2), 16);
      const b = parseInt(hex.substr(4, 2), 16);
      if (darkMode) {
        document.documentElement.style.setProperty('--accent-light', `rgba(${r},${g},${b},0.15)`);
      } else {
        document.documentElement.style.setProperty('--accent-light', `rgba(${r},${g},${b},0.08)`);
      }
    }
  }

  function toggleDarkMode() {
    darkMode = !darkMode;
    applyTheme();
  }

  // --- Settings ---
  function openSettings() {
    editTheme = { ...theme };
    showSettings = true;
    selectedSession = null;
    showNewCrawl = false;
  }

  function previewTheme() {
    theme.accent_color = editTheme.accent_color;
    theme.app_name = editTheme.app_name;
    theme.logo_url = editTheme.logo_url;
    darkMode = editTheme.mode === 'dark';
    applyTheme();
  }

  async function saveTheme() {
    savingTheme = true;
    try {
      const saved = await updateTheme(editTheme);
      theme = saved;
      darkMode = saved.mode === 'dark';
      applyTheme();
      showSettings = false;
    } catch (e) {
      error = e.message;
    } finally {
      savingTheme = false;
    }
  }

  function cancelSettings() {
    // Revert to saved theme
    loadTheme();
    showSettings = false;
  }

  // --- Filter helpers ---
  function applyFilters() {
    pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
    if (selectedSession) {
      pushURL(`/sessions/${selectedSession.ID}/${tab}`, filters);
    }
    loadTabData();
  }

  function clearFilters() {
    filters = {};
    pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
    if (selectedSession) {
      pushURL(`/sessions/${selectedSession.ID}/${tab}`);
    }
    loadTabData();
  }

  function setFilter(key, val) {
    filters[key] = val;
    filters = { ...filters };
  }

  function hasActiveFilters() {
    return Object.values(filters).some(v => v && v !== '');
  }

  // --- URL Routing ---
  function pushURL(path, queryFilters = {}, offset = 0) {
    const params = new URLSearchParams();
    for (const [k, v] of Object.entries(queryFilters)) {
      if (v !== '' && v != null) params.set(k, v);
    }
    if (offset > 0) params.set('offset', String(offset));
    const qs = params.toString();
    const full = qs ? `${path}?${qs}` : path;
    if (window.location.pathname + window.location.search !== full) {
      history.pushState(null, '', full);
    }
  }

  function parseRoute() {
    const path = window.location.pathname;
    const search = window.location.search;
    const urlMatch = path.match(/^\/sessions\/([^/]+)\/url\/(.+)/);
    if (urlMatch) {
      return { sessionId: urlMatch[1], tab: 'url-detail', detailUrl: decodeURIComponent(urlMatch[2]), filters: {}, offset: 0 };
    }
    const m = path.match(/^\/sessions\/([^/]+)(?:\/([^/]+))?/);
    if (m) {
      const sp = new URLSearchParams(search);
      const routeFilters = {};
      let routeOffset = 0;
      for (const [k, v] of sp.entries()) {
        if (k === 'offset') {
          routeOffset = parseInt(v, 10) || 0;
        } else if (k !== 'limit') {
          routeFilters[k] = v;
        }
      }
      return { sessionId: m[1], tab: m[2] || 'overview', filters: routeFilters, offset: routeOffset };
    }
    return null;
  }

  async function navigateTo(path, queryFilters = {}) {
    pushURL(path, queryFilters);
    await applyRoute();
  }

  async function applyRoute() {
    const route = parseRoute();
    if (route) {
      if (!selectedSession || selectedSession.ID !== route.sessionId) {
        if (sessions.length === 0) {
          await loadSessions();
        }
        const found = sessions.find(s => s.ID === route.sessionId);
        if (found) {
          selectedSession = found;
          stats = await getStats(found.ID);
          loadStorageStats();
        }
      }
      if (route.tab === 'url-detail') {
        tab = 'url-detail';
        filters = {};
        pageDetail = null;
        await loadPageDetail(route.sessionId, route.detailUrl);
      } else {
        tab = route.tab;
        pageDetail = null;
        filters = route.filters || {};
        const off = route.offset || 0;
        if (['internal'].includes(tab)) { intLinksOffset = off; }
        else if (['external'].includes(tab)) { extLinksOffset = off; }
        else { pagesOffset = off; }
        if (tab === 'pagerank') {
          await loadPRSubView(prSubView);
        } else {
          await loadTabData();
        }
      }
    } else {
      selectedSession = null;
      stats = null;
      pageDetail = null;
      filters = {};
      await loadSessions();
    }
  }

  window.addEventListener('popstate', () => applyRoute());

  async function selectSession(session) {
    selectedSession = session;
    tab = 'overview';
    filters = {};
    pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
    pushURL(`/sessions/${session.ID}/overview`);
    try {
      stats = await getStats(session.ID);
      loadStorageStats();
      await loadTabData();
    } catch (e) {
      error = e.message;
    }
  }

  function goHome() {
    selectedSession = null;
    stats = null;
    showNewCrawl = false;
    showSettings = false;
    pushURL('/');
  }

  async function loadSessions() {
    try {
      loading = true;
      sessions = await getSessions() || [];
      for (const s of sessions) {
        if (s.is_running && !sseConnections[s.ID]) {
          sseConnections[s.ID] = subscribeProgress(s.ID,
            (data) => { liveProgress[s.ID] = data; liveProgress = { ...liveProgress }; },
            () => { delete sseConnections[s.ID]; loadSessions(); }
          );
        }
      }
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function loadTabData() {
    if (!selectedSession) return;
    const id = selectedSession.ID;
    try {
      if (['overview','titles','meta','headings','images','indexability','response'].includes(tab)) {
        const result = await getPages(id, PAGE_SIZE, pagesOffset, filters);
        pages = result || [];
        hasMorePages = pages.length === PAGE_SIZE;
      } else if (tab === 'internal') {
        const result = await getInternalLinks(id, PAGE_SIZE, intLinksOffset, filters);
        intLinks = result || [];
        hasMoreIntLinks = intLinks.length === PAGE_SIZE;
      } else if (tab === 'external') {
        const result = await getExternalLinks(id, PAGE_SIZE, extLinksOffset, filters);
        extLinks = result || [];
        hasMoreExtLinks = extLinks.length === PAGE_SIZE;
      }
    } catch (e) {
      error = e.message;
    }
  }

  function switchTab(newTab) {
    tab = newTab;
    filters = {};
    pagesOffset = 0; extLinksOffset = 0; intLinksOffset = 0;
    if (selectedSession) {
      pushURL(`/sessions/${selectedSession.ID}/${newTab}`);
    }
    if (newTab === 'pagerank') {
      loadPRSubView(prSubView);
    } else if (newTab === 'robots') {
      loadRobotsHosts();
    } else {
      loadTabData();
    }
  }

  async function nextPage() {
    if (tab === 'internal') { intLinksOffset += PAGE_SIZE; }
    else if (tab === 'external') { extLinksOffset += PAGE_SIZE; }
    else { pagesOffset += PAGE_SIZE; }
    if (selectedSession) pushURL(`/sessions/${selectedSession.ID}/${tab}`, filters, currentOffset());
    await loadTabData();
  }
  async function prevPage() {
    if (tab === 'internal') { intLinksOffset = Math.max(0, intLinksOffset - PAGE_SIZE); }
    else if (tab === 'external') { extLinksOffset = Math.max(0, extLinksOffset - PAGE_SIZE); }
    else { pagesOffset = Math.max(0, pagesOffset - PAGE_SIZE); }
    if (selectedSession) pushURL(`/sessions/${selectedSession.ID}/${tab}`, filters, currentOffset());
    await loadTabData();
  }

  function currentOffset() {
    if (tab === 'internal') return intLinksOffset;
    if (tab === 'external') return extLinksOffset;
    return pagesOffset;
  }
  function currentData() {
    if (tab === 'internal') return intLinks;
    if (tab === 'external') return extLinks;
    return pages;
  }
  function hasMore() {
    if (tab === 'internal') return hasMoreIntLinks;
    if (tab === 'external') return hasMoreExtLinks;
    return hasMorePages;
  }

  async function handleStartCrawl() {
    const seeds = seedInput.split('\n').map(s => s.trim()).filter(Boolean);
    if (seeds.length === 0) return;
    starting = true;
    error = null;
    try {
      await startCrawl(seeds, { max_pages: maxPages, max_depth: maxDepth, workers, delay: crawlDelay, store_html: storeHtml });
      showNewCrawl = false;
      seedInput = '';
      maxPages = 0;
      maxDepth = 0;
      setTimeout(() => loadSessions(), 500);
    } catch (e) {
      error = e.message;
    } finally {
      starting = false;
    }
  }

  async function handleStop(id) {
    try {
      await stopCrawl(id);
      setTimeout(async () => {
        await loadSessions();
        const sess = sessions.find(s => s.ID === id);
        if (sess && selectedSession?.ID !== id) {
          selectSession(sess);
        }
      }, 1000);
    } catch (e) { error = e.message; }
  }

  function openResumeModal(id) {
    const sess = sessions.find(s => s.ID === id);
    let cfg = {};
    if (sess?.Config) {
      try { cfg = typeof sess.Config === 'string' ? JSON.parse(sess.Config) : sess.Config; } catch {}
    }
    resumeSessionId = id;
    resumeMaxPages = cfg.max_pages || 0;
    resumeMaxDepth = cfg.max_depth || 0;
    resumeWorkers = cfg.workers || 10;
    resumeDelay = cfg.delay || '1s';
    resumeStoreHtml = cfg.store_html || false;
    showResumeModal = true;
  }

  function closeResumeModal() {
    showResumeModal = false;
    resumeSessionId = null;
  }

  async function handleResume() {
    resuming = true;
    error = null;
    try {
      await resumeCrawl(resumeSessionId, {
        max_pages: resumeMaxPages,
        max_depth: resumeMaxDepth,
        workers: resumeWorkers,
        delay: resumeDelay,
        store_html: resumeStoreHtml,
      });
      closeResumeModal();
      await loadSessions();
      const sess = sessions.find(s => s.ID === resumeSessionId);
      if (sess) {
        await selectSession(sess);
      }
    } catch (e) { error = e.message; }
    finally { resuming = false; }
  }

  async function handleDelete(id) {
    if (!confirm('Delete this session and all its data?')) return;
    try {
      await deleteSession(id);
      if (selectedSession?.ID === id) { selectedSession = null; pushURL('/'); }
      loadSessions();
    } catch (e) { error = e.message; }
  }

  let retryingFailed = $state(false);
  async function handleRetryFailed(id) {
    retryingFailed = true;
    error = null;
    try {
      const result = await retryFailed(id);
      setTimeout(() => { loadSessions(); if (selectedSession) selectSession(selectedSession); }, 2000);
    } catch (e) { error = e.message; }
    finally { retryingFailed = false; }
  }

  let recomputing = $state(false);
  async function handleRecomputeDepths(id) {
    recomputing = true;
    error = null;
    try {
      await recomputeDepths(id);
      await selectSession(selectedSession);
    } catch (e) { error = e.message; }
    finally { recomputing = false; }
  }

  let computingPR = $state(false);
  async function handleComputePageRank(id) {
    computingPR = true;
    error = null;
    try {
      await computePageRank(id);
      await selectSession(selectedSession);
    } catch (e) { error = e.message; }
    finally { computingPR = false; }
  }

  function statusBadge(code) {
    if (code >= 200 && code < 300) return 'badge-success';
    if (code >= 300 && code < 400) return 'badge-info';
    if (code >= 400 && code < 500) return 'badge-warning';
    return 'badge-error';
  }

  function fmt(ms) { return ms < 1000 ? `${ms}ms` : `${(ms/1000).toFixed(1)}s`; }
  function fmtSize(b) { return b < 1024 ? `${b}B` : b < 1048576 ? `${(b/1024).toFixed(1)}KB` : `${(b/1048576).toFixed(1)}MB`; }
  function fmtN(n) { return new Intl.NumberFormat().format(n); }
  function trunc(s, n) { return s && s.length > n ? s.slice(0, n) + '...' : (s || '-'); }
  function timeAgo(date) {
    const d = new Date(date);
    const now = new Date();
    const diff = Math.floor((now - d) / 1000);
    if (diff < 60) return 'just now';
    if (diff < 3600) return `${Math.floor(diff/60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff/3600)}h ago`;
    return d.toLocaleDateString();
  }

  async function openHtmlModal(url) {
    htmlModalLoading = true;
    showHtmlModal = true;
    htmlModalView = 'render';
    try {
      htmlModalData = await getPageHTML(selectedSession.ID, url);
    } catch (e) {
      htmlModalData = { url, body_html: '' };
      error = e.message;
    } finally {
      htmlModalLoading = false;
    }
  }

  function closeHtmlModal() {
    showHtmlModal = false;
    htmlModalData = { url: '', body_html: '' };
  }

  let inLinksPage = $state(0);
  const IN_LINKS_PER_PAGE = 100;

  async function loadPageDetail(sessionId, url, inOffset = 0) {
    pageDetailLoading = true;
    try {
      pageDetail = await getPageDetail(sessionId, url, IN_LINKS_PER_PAGE, inOffset);
      inLinksPage = Math.floor(inOffset / IN_LINKS_PER_PAGE);
    } catch (e) {
      error = e.message;
    } finally {
      pageDetailLoading = false;
    }
  }

  async function loadInLinksPage(offset) {
    if (!pageDetail?.page || !selectedSession) return;
    try {
      const data = await getPageDetail(selectedSession.ID, pageDetail.page.URL, IN_LINKS_PER_PAGE, offset);
      pageDetail = { ...pageDetail, links: { ...pageDetail.links, in_links: data.links.in_links } };
      inLinksPage = Math.floor(offset / IN_LINKS_PER_PAGE);
    } catch (e) { error = e.message; }
  }

  function urlDetailHref(url) {
    if (!selectedSession) return '#';
    return `/sessions/${selectedSession.ID}/url/${encodeURIComponent(url)}`;
  }

  function goToUrlDetail(e, url) {
    e.preventDefault();
    navigateTo(urlDetailHref(url));
  }

  async function loadStorageStats() {
    try {
      storageStats = await getStorageStats();
    } catch {}
  }

  async function loadSystemStats() {
    try {
      systemStats = await getSystemStats();
    } catch {}
  }

  function startSystemStatsPolling() {
    if (systemStatsInterval) return;
    loadSystemStats();
    systemStatsInterval = setInterval(loadSystemStats, 3000);
  }

  // --- PageRank data loaders ---
  async function loadPRSubView(view) {
    if (!selectedSession) return;
    prLoading = true;
    const id = selectedSession.ID;
    try {
      if (view === 'top') {
        prTopData = await getPageRankTop(id, prTopLimit, prTopOffset);
      } else if (view === 'directory') {
        prTreemapData = await getPageRankTreemap(id, prTreemapDepth, prTreemapMinPages);
      } else if (view === 'distribution') {
        prDistData = await getPageRankDistribution(id, 20);
      } else if (view === 'table') {
        prTableData = await getPageRankTop(id, 50, prTableOffset, prTableDir);
      }
    } catch (e) {
      error = e.message;
    } finally {
      prLoading = false;
    }
  }

  function switchPRSubView(view) {
    prSubView = view;
    if (view === 'top') { prTopOffset = 0; }
    if (view === 'table') { prTableOffset = 0; }
    loadPRSubView(view);
  }

  function prDrillToTable(dir) {
    prTableDir = dir;
    prTableOffset = 0;
    prSubView = 'table';
    loadPRSubView('table');
  }

  function prDrillHistToTable(minPR, maxPR) {
    // We use directory filter as a PR range indicator — but since our API uses directory prefix,
    // we switch to table view; the user sees all pages sorted by PR which effectively shows the range
    prTableDir = '';
    prTableOffset = 0;
    prSubView = 'table';
    loadPRSubView('table');
  }

  // Squarified treemap layout algorithm
  function squarify(items, x, y, w, h) {
    if (items.length === 0 || w <= 0 || h <= 0) return [];
    const totalValue = items.reduce((s, it) => s + it.value, 0);
    if (totalValue <= 0) return [];
    const rects = [];
    let remaining = [...items];
    let cx = x, cy = y, cw = w, ch = h;

    while (remaining.length > 0) {
      const isWide = cw >= ch;
      const side = isWide ? ch : cw;
      const totalRemaining = remaining.reduce((s, it) => s + it.value, 0);
      let row = [remaining[0]];
      let rowValue = remaining[0].value;

      const worstRatio = (rv, s) => {
        const area = (rv / totalRemaining) * cw * ch;
        const rowLen = area / s;
        return Math.max(s / rowLen, rowLen / s);
      };

      for (let i = 1; i < remaining.length; i++) {
        const newRowValue = rowValue + remaining[i].value;
        const newArea = (newRowValue / totalRemaining) * cw * ch;
        const oldArea = (rowValue / totalRemaining) * cw * ch;
        const newSide = isWide ? newArea / ch : newArea / cw;
        const oldSide = isWide ? oldArea / ch : oldArea / cw;

        const oldWorst = Math.max(...row.map(it => {
          const a = (it.value / rowValue) * oldArea;
          const r = oldSide > 0 ? Math.max(a / (oldSide * oldSide) * oldSide, oldSide / (a / oldSide)) : Infinity;
          return Math.max(oldSide / (a / oldSide), (a / oldSide) / oldSide);
        }));
        const newRow = [...row, remaining[i]];
        const newWorst = Math.max(...newRow.map(it => {
          const a = (it.value / newRowValue) * newArea;
          return Math.max(newSide / (a / newSide), (a / newSide) / newSide);
        }));

        if (newWorst <= oldWorst) {
          row.push(remaining[i]);
          rowValue = newRowValue;
        } else {
          break;
        }
      }

      // Lay out the row
      const rowArea = (rowValue / totalRemaining) * cw * ch;
      const rowSide = isWide ? (ch > 0 ? rowArea / ch : 0) : (cw > 0 ? rowArea / cw : 0);
      let offset = 0;
      for (const item of row) {
        const fraction = rowValue > 0 ? item.value / rowValue : 0;
        const itemLen = fraction * (isWide ? ch : cw);
        rects.push({
          ...item,
          x: isWide ? cx : cx + offset,
          y: isWide ? cy + offset : cy,
          w: isWide ? rowSide : itemLen,
          h: isWide ? itemLen : rowSide,
        });
        offset += itemLen;
      }

      // Reduce remaining area
      if (isWide) { cx += rowSide; cw -= rowSide; }
      else { cy += rowSide; ch -= rowSide; }
      remaining = remaining.slice(row.length);
    }
    return rects;
  }

  // --- Robots.txt ---
  async function loadRobotsHosts() {
    if (!selectedSession) return;
    robotsLoading = true;
    robotsSelectedHost = null;
    robotsContent = null;
    robotsTestResults = null;
    try {
      robotsHosts = await getRobotsHosts(selectedSession.ID);
    } catch (e) {
      robotsHosts = [];
    } finally {
      robotsLoading = false;
    }
  }

  async function selectRobotsHost(host) {
    if (!selectedSession) return;
    robotsSelectedHost = host;
    robotsContent = null;
    robotsTestResults = null;
    robotsLoading = true;
    try {
      const data = await getRobotsContent(selectedSession.ID, host);
      robotsContent = data.Content || data.content || '';
    } catch (e) {
      robotsContent = '(failed to load)';
    } finally {
      robotsLoading = false;
    }
  }

  async function runRobotsTest() {
    if (!selectedSession || !robotsSelectedHost) return;
    const urls = robotsTestUrls.split('\n').map(u => u.trim()).filter(Boolean);
    if (urls.length === 0) return;
    robotsTestLoading = true;
    try {
      const data = await testRobotsUrls(selectedSession.ID, robotsSelectedHost, robotsTestUA || '*', urls);
      robotsTestResults = data.results;
    } catch (e) {
      error = e.message;
    } finally {
      robotsTestLoading = false;
    }
  }

  const TABS = [
    { id: 'overview', label: 'All Pages' },
    { id: 'titles', label: 'Titles' },
    { id: 'meta', label: 'Meta' },
    { id: 'headings', label: 'H1/H2' },
    { id: 'images', label: 'Images' },
    { id: 'indexability', label: 'Indexability' },
    { id: 'response', label: 'Response' },
    { id: 'internal', label: 'Internal Links' },
    { id: 'external', label: 'External Links' },
    { id: 'pagerank', label: 'PageRank' },
    { id: 'robots', label: 'Robots.txt' },
    { id: 'stats', label: 'Stats' },
  ];

  // Boot
  loadTheme();
  applyRoute();
  startSystemStatsPolling();
</script>

<div class="layout">
  <!-- Sidebar -->
  <aside class="sidebar">
    <div class="sidebar-header">
      {#if theme.logo_url}
        <img class="sidebar-logo" src={theme.logo_url} alt="Logo" />
      {:else}
        <div class="sidebar-logo-placeholder">
          <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
        </div>
      {/if}
      <span class="sidebar-app-name">{theme.app_name}</span>
    </div>

    <div class="sidebar-section">
      <div class="sidebar-section-title">Main Menu</div>
      <nav class="sidebar-nav">
        <button class="sidebar-link" class:active={!selectedSession && !showNewCrawl} onclick={() => goHome()}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/></svg>
          Dashboard
        </button>
        <button class="sidebar-link" class:active={showNewCrawl && !selectedSession} onclick={() => { goHome(); showNewCrawl = true; }}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="16"/><line x1="8" y1="12" x2="16" y2="12"/></svg>
          New Crawl
        </button>
      </nav>
    </div>

    {#if sessions.length > 0}
      <div class="sidebar-section">
        <div class="sidebar-section-title">Recent Sessions</div>
        <nav class="sidebar-nav">
          {#each sessions.slice(0, 8) as s}
            <button class="sidebar-link" class:active={selectedSession?.ID === s.ID} onclick={() => selectSession(s)}>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/></svg>
              <span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                {#if s.is_running}
                  <span style="color: var(--info);">{new URL(s.SeedURLs?.[0] || 'https://unknown').hostname}</span>
                {:else}
                  {new URL(s.SeedURLs?.[0] || 'https://unknown').hostname}
                {/if}
              </span>
            </button>
          {/each}
        </nav>
      </div>
    {/if}

    {#if systemStats}
      <div class="sidebar-section">
        <div class="sidebar-section-title">System</div>
        <div class="sidebar-stats">
          <div class="sidebar-stat">
            <span class="sidebar-stat-label">Memory</span>
            <span class="sidebar-stat-value">{fmtSize(systemStats.mem_alloc)}</span>
          </div>
          <div class="sidebar-stat">
            <span class="sidebar-stat-label">Heap</span>
            <span class="sidebar-stat-value">{fmtSize(systemStats.mem_heap_inuse)}</span>
          </div>
          <div class="sidebar-stat">
            <span class="sidebar-stat-label">Sys</span>
            <span class="sidebar-stat-value">{fmtSize(systemStats.mem_sys)}</span>
          </div>
          <div class="sidebar-stat">
            <span class="sidebar-stat-label">Goroutines</span>
            <span class="sidebar-stat-value">{fmtN(systemStats.num_goroutines)}</span>
          </div>
          <div class="sidebar-stat">
            <span class="sidebar-stat-label">GC cycles</span>
            <span class="sidebar-stat-value">{fmtN(systemStats.num_gc)}</span>
          </div>
        </div>
      </div>
    {/if}

    <div class="sidebar-section" style="margin-top: auto;">
      <div class="sidebar-section-title">General</div>
      <nav class="sidebar-nav">
        <button class="sidebar-link" class:active={showSettings} onclick={openSettings}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
          Settings
        </button>
      </nav>
    </div>

    <div class="sidebar-footer">
      <button class="theme-toggle" onclick={toggleDarkMode}>
        {#if darkMode}
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
          Light mode
        {:else}
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
          Dark mode
        {/if}
      </button>
    </div>
  </aside>

  <!-- Main Content -->
  <main class="main">
    <div class="main-content">
      {#if error}
        <div class="alert alert-error">
          <span>{error}</span>
          <button class="btn btn-sm btn-ghost" onclick={() => error = null}>Dismiss</button>
        </div>
      {/if}

      {#if showSettings}
        <!-- Settings -->
        <div class="page-header">
          <h1>Settings</h1>
        </div>
        <div class="card">
          <div class="form-grid">
            <div class="form-group">
              <label for="set-appname">App Name</label>
              <input id="set-appname" type="text" bind:value={editTheme.app_name} oninput={previewTheme} />
            </div>
            <div class="form-group">
              <label for="set-logo">Logo URL</label>
              <input id="set-logo" type="text" bind:value={editTheme.logo_url} oninput={previewTheme} placeholder="https://example.com/logo.png" />
            </div>
            {#if editTheme.logo_url}
              <div class="form-group" style="grid-column: 1 / -1;">
                <span style="font-weight: 500; font-size: 0.85rem; color: var(--text-secondary);">Logo Preview</span>
                <img src={editTheme.logo_url} alt="Logo preview" style="max-height: 48px; border-radius: 6px; background: var(--bg-card);" />
              </div>
            {/if}
            <div class="form-group">
              <label for="set-accent">Accent Color</label>
              <div style="display: flex; align-items: center; gap: 10px;">
                <input id="set-accent" type="color" value={editTheme.accent_color} oninput={(e) => { editTheme.accent_color = e.target.value; previewTheme(); }} style="width: 48px; height: 36px; border: 1px solid var(--border); border-radius: 6px; cursor: pointer; padding: 2px;" />
                <span style="font-family: monospace; color: var(--text-secondary);">{editTheme.accent_color}</span>
              </div>
            </div>
            <div class="form-group">
              <span style="font-weight: 500; font-size: 0.85rem; color: var(--text-secondary); display: block; margin-bottom: 4px;">Mode</span>
              <div style="display: flex; gap: 8px;">
                <button class="btn btn-sm" class:btn-primary={editTheme.mode === 'light'} onclick={() => { editTheme.mode = 'light'; previewTheme(); }}>Light</button>
                <button class="btn btn-sm" class:btn-primary={editTheme.mode === 'dark'} onclick={() => { editTheme.mode = 'dark'; previewTheme(); }}>Dark</button>
              </div>
            </div>
          </div>
          <div style="display: flex; gap: 8px; margin-top: 20px;">
            <button class="btn btn-primary" onclick={saveTheme} disabled={savingTheme}>
              {savingTheme ? 'Saving...' : 'Save'}
            </button>
            <button class="btn" onclick={cancelSettings}>Cancel</button>
          </div>
        </div>

      {:else if showNewCrawl && !selectedSession}
        <!-- New Crawl Form -->
        <div class="page-header">
          <h1>New Crawl</h1>
        </div>
        <div class="card">
          <div class="form-grid">
            <div class="form-group" style="grid-column: 1 / -1;">
              <label for="seeds">Seed URLs (one per line)</label>
              <textarea id="seeds" bind:value={seedInput} rows="3" placeholder="https://example.com"></textarea>
            </div>
            <div class="form-group"><label for="workers">Workers</label><input id="workers" type="number" bind:value={workers} min="1" max="100" /></div>
            <div class="form-group"><label for="delay">Delay</label><input id="delay" type="text" bind:value={crawlDelay} placeholder="1s" /></div>
            <div class="form-group"><label for="maxpages">Max pages (0 = unlimited)</label><input id="maxpages" type="number" bind:value={maxPages} min="0" /></div>
            <div class="form-group"><label for="maxdepth">Max depth (0 = unlimited)</label><input id="maxdepth" type="number" bind:value={maxDepth} min="0" /></div>
            <div class="form-group" style="display: flex; flex-direction: row; align-items: center; gap: 8px; padding-top: 24px;">
              <input id="storehtml" type="checkbox" bind:checked={storeHtml} /><label for="storehtml" style="margin: 0;">Store raw HTML</label>
            </div>
          </div>
          <div style="display: flex; gap: 8px; margin-top: 20px;">
            <button class="btn btn-primary" onclick={handleStartCrawl} disabled={starting || !seedInput.trim()}>
              {starting ? 'Starting...' : 'Start Crawl'}
            </button>
            <button class="btn" onclick={() => showNewCrawl = false}>Cancel</button>
          </div>
        </div>

      {:else if !selectedSession}
        <!-- Sessions List -->
        <div class="page-header">
          <h1>Crawl Sessions</h1>
          <button class="btn btn-primary" onclick={() => showNewCrawl = true}>
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
            New Crawl
          </button>
        </div>

        {#if loading}
          <p style="color: var(--text-muted); padding: 40px 0;">Loading...</p>
        {:else if sessions.length === 0}
          <div class="empty-state">
            <h2>No crawl sessions yet</h2>
            <p>Start your first crawl to begin analyzing your site.</p>
            <button class="btn btn-primary" style="margin-top: 16px;" onclick={() => showNewCrawl = true}>Start a Crawl</button>
          </div>
        {:else}
          <div class="card card-flush">
            {#each sessions as s}
              <div class="session-row">
                <div class="session-info">
                  <div class="session-seed">{s.SeedURLs?.[0] || 'Unknown'}</div>
                  <div class="session-meta">
                    {#if s.is_running}
                      <span class="badge badge-info">
                        Running
                        {#if liveProgress[s.ID]}
                          &middot; {fmtN(liveProgress[s.ID].pages_crawled)} pages &middot; {fmtN(liveProgress[s.ID].queue_size)} queued
                        {/if}
                      </span>
                    {:else}
                      <span class="badge" class:badge-success={s.Status==='completed'} class:badge-error={s.Status==='failed'} class:badge-warning={s.Status==='stopped'}>{s.Status}</span>
                    {/if}
                    <span>{fmtN(s.PagesCrawled)} pages</span>
                    <span>{timeAgo(s.StartedAt)}</span>
                  </div>
                </div>
                <div class="session-actions">
                  <button class="btn btn-sm" onclick={() => selectSession(s)}>View</button>
                  {#if s.is_running}
                    <button class="btn btn-sm btn-danger" onclick={() => handleStop(s.ID)}>Stop</button>
                  {:else}
                    <button class="btn btn-sm" onclick={() => openResumeModal(s.ID)}>Resume</button>
                    <button class="btn btn-sm btn-danger" onclick={() => handleDelete(s.ID)}>Delete</button>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        {/if}

      {:else if tab === 'url-detail' && selectedSession}
        <!-- URL Detail View -->
        <div class="breadcrumb">
          <a href="/" onclick={(e) => { e.preventDefault(); goHome(); }}>Sessions</a>
          <span>/</span>
          <a href={`/sessions/${selectedSession.ID}/overview`} onclick={(e) => { e.preventDefault(); navigateTo(`/sessions/${selectedSession.ID}/overview`); }}>{selectedSession.SeedURLs?.[0] || selectedSession.ID}</a>
          <span>/</span>
          <span style="color: var(--text);">URL Detail</span>
        </div>

        {#if pageDetailLoading}
          <p style="color: var(--text-muted); padding: 40px 0;">Loading...</p>
        {:else if pageDetail?.page}
          {@const pg = pageDetail.page}
          {@const outLinks = pageDetail.links?.out_links || []}
          {@const inLinks = pageDetail.links?.in_links || []}
          {@const outLinksCount = pageDetail.links?.out_links_count || 0}
          {@const inLinksCount = pageDetail.links?.in_links_count || 0}

          <!-- Header -->
          <div class="page-header" style="gap: 12px; flex-wrap: wrap;">
            <div style="display: flex; align-items: center; gap: 8px; min-width: 0; flex: 1;">
              <button class="btn btn-sm" onclick={() => navigateTo(`/sessions/${selectedSession.ID}/overview`)} title="Back">
                <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
              </button>
              <h1 style="font-size: 1rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title={pg.URL}>{pg.URL}</h1>
              <span class="badge {statusBadge(pg.StatusCode)}">{pg.StatusCode}</span>
            </div>
            <div style="display: flex; gap: 6px;">
              <a class="btn btn-sm" href={pg.URL} target="_blank" rel="noopener">Open URL</a>
              {#if pg.BodySize > 0}
                <button class="btn btn-sm" onclick={() => openHtmlModal(pg.URL)}>View HTML</button>
              {/if}
            </div>
          </div>

          <!-- Summary Cards -->
          <div class="stats-grid">
            <div class="stat-card"><div class="stat-value"><span class="badge {statusBadge(pg.StatusCode)}" style="font-size: 1.2rem;">{pg.StatusCode}</span></div><div class="stat-label">Status Code</div></div>
            <div class="stat-card"><div class="stat-value" style="font-size: 0.95rem;">{pg.ContentType || '-'}</div><div class="stat-label">Content-Type</div></div>
            <div class="stat-card"><div class="stat-value">{fmtSize(pg.BodySize)}</div><div class="stat-label">Size</div></div>
            <div class="stat-card"><div class="stat-value">{fmt(pg.FetchDurationMs)}</div><div class="stat-label">Response Time</div></div>
            <div class="stat-card"><div class="stat-value">{pg.Depth}</div><div class="stat-label">Depth</div></div>
            {#if pg.PageRank > 0}
              <div class="stat-card"><div class="stat-value" style="color: var(--accent)">{pg.PageRank.toFixed(1)}</div><div class="stat-label">PageRank</div></div>
            {/if}
            {#if pg.FoundOn}
              <div class="stat-card"><div class="stat-value" style="font-size: 0.8rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;"><a href={urlDetailHref(pg.FoundOn)} onclick={(e) => goToUrlDetail(e, pg.FoundOn)} style="color: var(--accent);">{pg.FoundOn}</a></div><div class="stat-label">Found On</div></div>
            {/if}
            <div class="stat-card"><div class="stat-value" style="font-size: 0.8rem;">{new Date(pg.CrawledAt).toLocaleString()}</div><div class="stat-label">Crawled At</div></div>
          </div>

          {#if pg.Error}
            <div class="alert alert-error" style="margin-bottom: 16px;">
              <strong>Error:</strong> {pg.Error}
            </div>
          {/if}

          <!-- Response Headers -->
          {#if pg.Headers && Object.keys(pg.Headers).length > 0}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Response Headers</h3>
              <table>
                <thead><tr><th>Header</th><th>Value</th></tr></thead>
                <tbody>
                  {#each Object.entries(pg.Headers).sort((a,b) => a[0].localeCompare(b[0])) as [key, val]}
                    <tr><td style="font-weight: 500; white-space: nowrap;">{key}</td><td style="word-break: break-all;">{val}</td></tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}

          <!-- SEO -->
          <div class="card" style="margin-bottom: 16px;">
            <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">SEO</h3>
            <table>
              <tbody>
                <tr><td style="font-weight: 500; width: 160px;">Title</td><td>{pg.Title || '-'} <span style="color: var(--text-muted);">({pg.TitleLength} chars)</span></td></tr>
                <tr><td style="font-weight: 500;">Meta Description</td><td>{pg.MetaDescription || '-'} <span style="color: var(--text-muted);">({pg.MetaDescLength} chars)</span></td></tr>
                {#if pg.MetaKeywords}<tr><td style="font-weight: 500;">Meta Keywords</td><td>{pg.MetaKeywords}</td></tr>{/if}
                <tr><td style="font-weight: 500;">Meta Robots</td><td>{pg.MetaRobots || '-'}</td></tr>
                {#if pg.XRobotsTag}<tr><td style="font-weight: 500;">X-Robots-Tag</td><td>{pg.XRobotsTag}</td></tr>{/if}
                <tr><td style="font-weight: 500;">Canonical</td><td>{pg.Canonical || '-'} {#if pg.CanonicalIsSelf}<span class="badge badge-success" style="font-size: 0.7rem;">self</span>{/if}</td></tr>
                <tr><td style="font-weight: 500;">Indexable</td><td><span class="badge" class:badge-success={pg.IsIndexable} class:badge-error={!pg.IsIndexable}>{pg.IsIndexable ? 'Yes' : 'No'}</span> {#if pg.IndexReason}<span style="color: var(--text-muted);">({pg.IndexReason})</span>{/if}</td></tr>
                {#if pg.Lang}<tr><td style="font-weight: 500;">Language</td><td>{pg.Lang}</td></tr>{/if}
              </tbody>
            </table>
          </div>

          <!-- Open Graph -->
          {#if pg.OGTitle || pg.OGDescription || pg.OGImage}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Open Graph</h3>
              <table>
                <tbody>
                  {#if pg.OGTitle}<tr><td style="font-weight: 500; width: 160px;">OG Title</td><td>{pg.OGTitle}</td></tr>{/if}
                  {#if pg.OGDescription}<tr><td style="font-weight: 500;">OG Description</td><td>{pg.OGDescription}</td></tr>{/if}
                  {#if pg.OGImage}
                    <tr><td style="font-weight: 500;">OG Image</td><td><a href={pg.OGImage} target="_blank" rel="noopener">{pg.OGImage}</a></td></tr>
                    <tr><td></td><td><img src={pg.OGImage} alt="OG preview" style="max-width: 300px; max-height: 200px; border-radius: 6px; border: 1px solid var(--border); margin-top: 4px;" /></td></tr>
                  {/if}
                </tbody>
              </table>
            </div>
          {/if}

          <!-- Headings -->
          {#if pg.H1?.length || pg.H2?.length || pg.H3?.length || pg.H4?.length || pg.H5?.length || pg.H6?.length}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Headings</h3>
              {#each [['H1', pg.H1], ['H2', pg.H2], ['H3', pg.H3], ['H4', pg.H4], ['H5', pg.H5], ['H6', pg.H6]] as [label, items]}
                {#if items?.length}
                  <div style="margin-bottom: 8px;">
                    <strong style="font-size: 0.85rem;">{label}</strong> <span style="color: var(--text-muted);">({items.length})</span>
                    <ul style="margin: 4px 0 0 20px; padding: 0;">
                      {#each items as h}<li style="font-size: 0.85rem; color: var(--text-secondary);">{h}</li>{/each}
                    </ul>
                  </div>
                {/if}
              {/each}
            </div>
          {/if}

          <!-- Redirect Chain -->
          {#if pg.RedirectChain?.length}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Redirect Chain</h3>
              <table>
                <thead><tr><th>#</th><th>URL</th><th>Status</th></tr></thead>
                <tbody>
                  {#each pg.RedirectChain as hop, i}
                    <tr>
                      <td>{i + 1}</td>
                      <td class="cell-url">{hop.URL}</td>
                      <td><span class="badge {statusBadge(hop.StatusCode)}">{hop.StatusCode}</span></td>
                    </tr>
                  {/each}
                  <tr>
                    <td>{pg.RedirectChain.length + 1}</td>
                    <td class="cell-url" style="font-weight: 500;">{pg.FinalURL || pg.URL}</td>
                    <td><span class="badge {statusBadge(pg.StatusCode)}">{pg.StatusCode}</span></td>
                  </tr>
                </tbody>
              </table>
            </div>
          {/if}

          <!-- Hreflang -->
          {#if pg.Hreflang?.length}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Hreflang</h3>
              <table>
                <thead><tr><th>Language</th><th>URL</th></tr></thead>
                <tbody>
                  {#each pg.Hreflang as h}
                    <tr><td style="font-weight: 500;">{h.Lang}</td><td class="cell-url">{h.URL}</td></tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}

          <!-- Schema.org -->
          {#if pg.SchemaTypes?.length}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Schema.org</h3>
              <div style="display: flex; flex-wrap: wrap; gap: 6px;">
                {#each pg.SchemaTypes as t}
                  <span class="badge badge-info">{t}</span>
                {/each}
              </div>
            </div>
          {/if}

          <!-- Content -->
          <div class="card" style="margin-bottom: 16px;">
            <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Content</h3>
            <div class="stats-grid">
              <div class="stat-card"><div class="stat-value">{fmtN(pg.WordCount)}</div><div class="stat-label">Words</div></div>
              <div class="stat-card"><div class="stat-value">{pg.ImagesCount}</div><div class="stat-label">Images</div></div>
              <div class="stat-card"><div class="stat-value" style={pg.ImagesNoAlt > 0 ? 'color: var(--warning)' : ''}>{pg.ImagesNoAlt}</div><div class="stat-label">Images without alt</div></div>
              <div class="stat-card"><div class="stat-value">{fmtN(pg.InternalLinksOut)}</div><div class="stat-label">Internal Links Out</div></div>
              <div class="stat-card"><div class="stat-value">{fmtN(pg.ExternalLinksOut)}</div><div class="stat-label">External Links Out</div></div>
            </div>
          </div>

          <!-- Outbound Links -->
          {#if outLinks.length > 0}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Outbound Links <span style="color: var(--text-muted);">({fmtN(outLinksCount)})</span></h3>
              <table>
                <thead><tr><th>Target URL</th><th>Anchor</th><th>Type</th><th>Tag</th><th>Rel</th></tr></thead>
                <tbody>
                  {#each outLinks as l}
                    <tr>
                      <td class="cell-url">
                        {#if l.IsInternal}
                          <a href={urlDetailHref(l.TargetURL)} onclick={(e) => goToUrlDetail(e, l.TargetURL)}>{l.TargetURL}</a>
                        {:else}
                          <a href={l.TargetURL} target="_blank" rel="noopener">{l.TargetURL}</a>
                        {/if}
                      </td>
                      <td class="cell-title">{l.AnchorText || '-'}</td>
                      <td><span class="badge" class:badge-success={l.IsInternal} class:badge-warning={!l.IsInternal}>{l.IsInternal ? 'Internal' : 'External'}</span></td>
                      <td>{l.Tag || '-'}</td>
                      <td>{l.Rel || '-'}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}

          <!-- Inbound Links -->
          {#if inLinksCount > 0}
            <div class="card" style="margin-bottom: 16px;">
              <h3 style="margin: 0 0 12px 0; font-size: 0.95rem;">Inbound Links <span style="color: var(--text-muted);">({fmtN(inLinksCount)})</span></h3>
              <table>
                <thead><tr><th>Source URL</th><th>Anchor</th><th>Tag</th><th>Rel</th></tr></thead>
                <tbody>
                  {#each inLinks as l}
                    <tr>
                      <td class="cell-url"><a href={urlDetailHref(l.SourceURL)} onclick={(e) => goToUrlDetail(e, l.SourceURL)}>{l.SourceURL}</a></td>
                      <td class="cell-title">{l.AnchorText || '-'}</td>
                      <td>{l.Tag || '-'}</td>
                      <td>{l.Rel || '-'}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
              {#if inLinksCount > IN_LINKS_PER_PAGE}
                <div style="display: flex; gap: 8px; align-items: center; margin-top: 12px; justify-content: center;">
                  <button class="btn btn-sm" disabled={inLinksPage === 0} onclick={() => loadInLinksPage((inLinksPage - 1) * IN_LINKS_PER_PAGE)}>Prev</button>
                  <span style="color: var(--text-muted); font-size: 0.85rem;">{inLinksPage * IN_LINKS_PER_PAGE + 1}–{Math.min((inLinksPage + 1) * IN_LINKS_PER_PAGE, inLinksCount)} of {fmtN(inLinksCount)}</span>
                  <button class="btn btn-sm" disabled={(inLinksPage + 1) * IN_LINKS_PER_PAGE >= inLinksCount} onclick={() => loadInLinksPage((inLinksPage + 1) * IN_LINKS_PER_PAGE)}>Next</button>
                </div>
              {/if}
            </div>
          {/if}
        {:else}
          <p style="color: var(--text-muted); padding: 40px 0;">Page not found.</p>
        {/if}

      {:else}
        <!-- Session Detail -->
        <div class="breadcrumb">
          <a href="/" onclick={(e) => { e.preventDefault(); goHome(); }}>Sessions</a>
          <span>/</span>
          <span style="color: var(--text);">{selectedSession.SeedURLs?.[0] || selectedSession.ID}</span>
        </div>

        <div class="action-bar">
          {#if selectedSession.is_running}
            <span class="badge badge-info">Running
              {#if liveProgress[selectedSession.ID]}
                &middot; {fmtN(liveProgress[selectedSession.ID].pages_crawled)} pages
              {/if}
            </span>
            <button class="btn btn-sm btn-danger" onclick={() => handleStop(selectedSession.ID)}>Stop</button>
          {:else}
            <span class="badge" class:badge-success={selectedSession.Status==='completed'} class:badge-error={selectedSession.Status==='failed'} class:badge-warning={selectedSession.Status==='stopped'}>{selectedSession.Status}</span>
            <button class="btn btn-sm" onclick={() => openResumeModal(selectedSession.ID)}>Resume</button>
            <button class="btn btn-sm" onclick={() => handleRecomputeDepths(selectedSession.ID)} disabled={recomputing}>
              {recomputing ? 'Recomputing...' : 'Recompute Depths'}
            </button>
            <button class="btn btn-sm" onclick={() => handleComputePageRank(selectedSession.ID)} disabled={computingPR}>
              {computingPR ? 'Computing...' : 'Compute PageRank'}
            </button>
            {#if stats?.status_codes?.[0] > 0}
              <button class="btn btn-sm" onclick={() => handleRetryFailed(selectedSession.ID)} disabled={retryingFailed} title="Retry {stats.status_codes[0]} failed pages (status 0)">
                {retryingFailed ? 'Retrying...' : `Retry Failed (${stats.status_codes[0]})`}
              </button>
            {/if}
            <button class="btn btn-sm btn-danger" onclick={() => handleDelete(selectedSession.ID)}>Delete</button>
          {/if}
          <button class="btn btn-sm" onclick={() => selectSession(selectedSession)}>
            <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
            Refresh
          </button>
        </div>

        {#if stats}
          <div class="stats-grid">
            <div class="stat-card"><div class="stat-value">{fmtN(stats.total_pages)}</div><div class="stat-label">Pages crawled</div></div>
            <div class="stat-card"><div class="stat-value">{fmtN(stats.internal_links)}</div><div class="stat-label">Internal links</div></div>
            <div class="stat-card"><div class="stat-value">{fmtN(stats.external_links)}</div><div class="stat-label">External links</div></div>
            <div class="stat-card"><div class="stat-value">{fmt(Math.round(stats.avg_fetch_ms))}</div><div class="stat-label">Avg response</div></div>
            <div class="stat-card"><div class="stat-value" style="color: var(--error)">{fmtN(stats.error_count)}</div><div class="stat-label">Errors</div></div>
            {#if stats.pages_per_second > 0}
              <div class="stat-card"><div class="stat-value">{stats.pages_per_second.toFixed(1)}</div><div class="stat-label">Pages/sec</div></div>
            {/if}
            {#if stats.crawl_duration_sec > 0}
              <div class="stat-card"><div class="stat-value">{stats.crawl_duration_sec < 60 ? stats.crawl_duration_sec.toFixed(0) + 's' : (stats.crawl_duration_sec / 60).toFixed(1) + 'min'}</div><div class="stat-label">Duration</div></div>
            {/if}
            {#if storageStats?.tables?.length}
              <div class="stat-card">
                <div class="stat-value">{fmtSize(storageStats.tables.reduce((a, t) => a + t.bytes_on_disk, 0))}</div>
                <div class="stat-label">Storage total</div>
              </div>
              {#each storageStats.tables as t}
                <div class="stat-card">
                  <div class="stat-value">{fmtSize(t.bytes_on_disk)}</div>
                  <div class="stat-label">{t.name} ({fmtN(t.rows)} rows)</div>
                </div>
              {/each}
            {/if}
          </div>
        {/if}

        <div class="tab-bar">
          {#each TABS as t}
            <button class="tab" class:tab-active={tab === t.id} onclick={() => switchTab(t.id)}>{t.label}</button>
          {/each}
        </div>

        <div class="card card-flush" style="border-top-left-radius: 0; border-top-right-radius: 0; border-top: none;">

          {#if tab === 'overview'}
            <table>
              <thead>
                <tr><th>URL</th><th>Status</th><th>Title</th><th>Words</th><th>Int Out</th><th>Ext Out</th><th>Size</th><th>Time</th><th>Depth</th><th>PR</th><th></th></tr>
                <tr class="filter-row">
                  {#each TAB_FILTERS.overview as key}
                    <td><input class="filter-input" placeholder={key} value={filters[key] || ''} oninput={(e) => setFilter(key, e.target.value)} onkeydown={(e) => e.key === 'Enter' && applyFilters()} /></td>
                  {/each}
                  <td>{#if hasActiveFilters()}<button class="btn-filter-clear" title="Clear filters" onclick={clearFilters}><svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg></button>{/if}</td>
                </tr>
              </thead>
              <tbody>
                {#each pages as p}
                  <tr>
                    <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
                    <td><span class="badge {statusBadge(p.StatusCode)}">{p.StatusCode}</span></td>
                    <td class="cell-title">{trunc(p.Title, 60)}</td>
                    <td>{fmtN(p.WordCount)}</td>
                    <td>{fmtN(p.InternalLinksOut)}</td>
                    <td>{fmtN(p.ExternalLinksOut)}</td>
                    <td>{fmtSize(p.BodySize)}</td>
                    <td>{fmt(p.FetchDurationMs)}</td>
                    <td>{p.Depth}</td>
                    <td style="color: var(--accent); font-weight: 500;">{p.PageRank > 0 ? p.PageRank.toFixed(1) : '-'}</td>
                    <td>
                      {#if p.BodySize > 0}
                        <button class="btn-html" title="View HTML" onclick={() => openHtmlModal(p.URL)}>
                          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
                        </button>
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>

          {:else if tab === 'titles'}
            <table>
              <thead>
                <tr><th>URL</th><th>Title</th><th>Length</th><th>H1</th></tr>
                <tr class="filter-row">
                  {#each TAB_FILTERS.titles as key}
                    <td><input class="filter-input" placeholder={key} value={filters[key] || ''} oninput={(e) => setFilter(key, e.target.value)} onkeydown={(e) => e.key === 'Enter' && applyFilters()} /></td>
                  {/each}
                </tr>
              </thead>
              <tbody>
                {#each pages as p}
                  <tr>
                    <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
                    <td class="cell-title" class:cell-warn={p.TitleLength === 0 || p.TitleLength > 60}>{p.Title || '-'}</td>
                    <td class:cell-warn={p.TitleLength === 0 || p.TitleLength > 60}>{p.TitleLength}</td>
                    <td class="cell-title">{p.H1?.[0] || '-'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>

          {:else if tab === 'meta'}
            <table>
              <thead>
                <tr><th>URL</th><th>Meta Description</th><th>Length</th><th>Meta Keywords</th><th>OG Title</th></tr>
                <tr class="filter-row">
                  {#each TAB_FILTERS.meta as key}
                    <td><input class="filter-input" placeholder={key} value={filters[key] || ''} oninput={(e) => setFilter(key, e.target.value)} onkeydown={(e) => e.key === 'Enter' && applyFilters()} /></td>
                  {/each}
                </tr>
              </thead>
              <tbody>
                {#each pages as p}
                  <tr>
                    <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
                    <td class="cell-title" class:cell-warn={p.MetaDescLength === 0 || p.MetaDescLength > 160}>{trunc(p.MetaDescription, 80)}</td>
                    <td class:cell-warn={p.MetaDescLength === 0 || p.MetaDescLength > 160}>{p.MetaDescLength}</td>
                    <td class="cell-title">{trunc(p.MetaKeywords, 60)}</td>
                    <td class="cell-title">{trunc(p.OGTitle, 60)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>

          {:else if tab === 'headings'}
            <table>
              <thead>
                <tr><th>URL</th><th>H1</th><th>H1 Count</th><th>H2</th><th>H2 Count</th></tr>
                <tr class="filter-row">
                  {#each TAB_FILTERS.headings as key}
                    <td><input class="filter-input" placeholder={key} value={filters[key] || ''} oninput={(e) => setFilter(key, e.target.value)} onkeydown={(e) => e.key === 'Enter' && applyFilters()} /></td>
                  {/each}
                  <td></td><td></td>
                </tr>
              </thead>
              <tbody>
                {#each pages as p}
                  <tr>
                    <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
                    <td class="cell-title" class:cell-warn={!p.H1?.length || p.H1.length > 1}>{p.H1?.[0] || '-'}</td>
                    <td class:cell-warn={!p.H1?.length || p.H1.length > 1}>{p.H1?.length || 0}</td>
                    <td class="cell-title">{p.H2?.[0] || '-'}</td>
                    <td>{p.H2?.length || 0}</td>
                  </tr>
                {/each}
              </tbody>
            </table>

          {:else if tab === 'images'}
            <table>
              <thead>
                <tr><th>URL</th><th>Images</th><th>Without Alt</th><th>Title</th><th>Words</th></tr>
                <tr class="filter-row">
                  {#each TAB_FILTERS.images as key}
                    <td><input class="filter-input" placeholder={key} value={filters[key] || ''} oninput={(e) => setFilter(key, e.target.value)} onkeydown={(e) => e.key === 'Enter' && applyFilters()} /></td>
                  {/each}
                </tr>
              </thead>
              <tbody>
                {#each pages as p}
                  <tr>
                    <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
                    <td>{p.ImagesCount}</td>
                    <td class:cell-warn={p.ImagesNoAlt > 0}>{p.ImagesNoAlt}</td>
                    <td class="cell-title">{trunc(p.Title, 50)}</td>
                    <td>{fmtN(p.WordCount)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>

          {:else if tab === 'indexability'}
            <table>
              <thead>
                <tr><th>URL</th><th>Indexable</th><th>Reason</th><th>Meta Robots</th><th>Canonical</th><th>Self</th></tr>
                <tr class="filter-row">
                  {#each TAB_FILTERS.indexability as key}
                    <td><input class="filter-input" placeholder={key} value={filters[key] || ''} oninput={(e) => setFilter(key, e.target.value)} onkeydown={(e) => e.key === 'Enter' && applyFilters()} /></td>
                  {/each}
                </tr>
              </thead>
              <tbody>
                {#each pages as p}
                  <tr>
                    <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
                    <td><span class="badge" class:badge-success={p.IsIndexable} class:badge-error={!p.IsIndexable}>{p.IsIndexable ? 'Yes' : 'No'}</span></td>
                    <td>{p.IndexReason || '-'}</td>
                    <td>{p.MetaRobots || '-'}</td>
                    <td class="cell-url">{trunc(p.Canonical, 60)}</td>
                    <td>{p.CanonicalIsSelf ? 'Yes' : '-'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>

          {:else if tab === 'response'}
            <table>
              <thead>
                <tr><th>URL</th><th>Status</th><th>Content Type</th><th>Encoding</th><th>Size</th><th>Time</th><th>Redirects</th></tr>
                <tr class="filter-row">
                  {#each TAB_FILTERS.response as key}
                    <td><input class="filter-input" placeholder={key} value={filters[key] || ''} oninput={(e) => setFilter(key, e.target.value)} onkeydown={(e) => e.key === 'Enter' && applyFilters()} /></td>
                  {/each}
                  <td></td>
                </tr>
              </thead>
              <tbody>
                {#each pages as p}
                  <tr>
                    <td class="cell-url"><a href={urlDetailHref(p.URL)} onclick={(e) => goToUrlDetail(e, p.URL)}>{p.URL}</a></td>
                    <td><span class="badge {statusBadge(p.StatusCode)}">{p.StatusCode}</span></td>
                    <td>{p.ContentType || '-'}</td>
                    <td>{p.ContentEncoding || '-'}</td>
                    <td>{fmtSize(p.BodySize)}</td>
                    <td>{fmt(p.FetchDurationMs)}</td>
                    <td>{p.FinalURL !== p.URL ? p.FinalURL : '-'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>

          {:else if tab === 'internal'}
            <table>
              <thead>
                <tr><th>Source</th><th>Target</th><th>Anchor Text</th><th>Tag</th></tr>
                <tr class="filter-row">
                  {#each TAB_FILTERS.internal as key}
                    <td><input class="filter-input" placeholder={key} value={filters[key] || ''} oninput={(e) => setFilter(key, e.target.value)} onkeydown={(e) => e.key === 'Enter' && applyFilters()} /></td>
                  {/each}
                </tr>
              </thead>
              <tbody>
                {#each intLinks as l}
                  <tr>
                    <td class="cell-url"><a href={urlDetailHref(l.SourceURL)} onclick={(e) => goToUrlDetail(e, l.SourceURL)}>{l.SourceURL}</a></td>
                    <td class="cell-url"><a href={urlDetailHref(l.TargetURL)} onclick={(e) => goToUrlDetail(e, l.TargetURL)}>{l.TargetURL}</a></td>
                    <td class="cell-title">{l.AnchorText || '-'}</td>
                    <td>{l.Tag}</td>
                  </tr>
                {/each}
              </tbody>
            </table>

          {:else if tab === 'external'}
            <table>
              <thead>
                <tr><th>Source</th><th>Target</th><th>Anchor Text</th><th>Rel</th></tr>
                <tr class="filter-row">
                  {#each TAB_FILTERS.external as key}
                    <td><input class="filter-input" placeholder={key} value={filters[key] || ''} oninput={(e) => setFilter(key, e.target.value)} onkeydown={(e) => e.key === 'Enter' && applyFilters()} /></td>
                  {/each}
                </tr>
              </thead>
              <tbody>
                {#each extLinks as l}
                  <tr>
                    <td class="cell-url"><a href={urlDetailHref(l.SourceURL)} onclick={(e) => goToUrlDetail(e, l.SourceURL)}>{l.SourceURL}</a></td>
                    <td class="cell-url"><a href={l.TargetURL} target="_blank" rel="noopener">{l.TargetURL}</a></td>
                    <td class="cell-title">{l.AnchorText || '-'}</td>
                    <td>{l.Rel || '-'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>

          {:else if tab === 'pagerank'}
            <div class="pr-container">
              <div class="pr-subview-bar">
                <button class="pr-subview-btn" class:pr-subview-active={prSubView === 'top'} onclick={() => switchPRSubView('top')}>Top Pages</button>
                <button class="pr-subview-btn" class:pr-subview-active={prSubView === 'directory'} onclick={() => switchPRSubView('directory')}>By Directory</button>
                <button class="pr-subview-btn" class:pr-subview-active={prSubView === 'distribution'} onclick={() => switchPRSubView('distribution')}>Distribution</button>
                <button class="pr-subview-btn" class:pr-subview-active={prSubView === 'table'} onclick={() => switchPRSubView('table')}>Full Table</button>
              </div>

              {#if prLoading}
                <p style="color: var(--text-muted); padding: 40px 0; text-align: center;">Loading...</p>

              {:else if prSubView === 'top'}
                <!-- Top Pages bar chart -->
                {#if prTopData?.pages?.length > 0}
                  <div class="pr-controls">
                    <label>Show</label>
                    <select class="pr-select" value={prTopLimit} onchange={(e) => { prTopLimit = Number(e.target.value); prTopOffset = 0; loadPRSubView('top'); }}>
                      <option value={20}>20</option>
                      <option value={50}>50</option>
                      <option value={100}>100</option>
                    </select>
                    <span style="color: var(--text-muted); font-size: 12px;">of {fmtN(prTopData.total)} pages with PR</span>
                  </div>
                  {@const maxPR = prTopData.pages[0]?.pagerank || 1}
                  {#each prTopData.pages as p, i}
                    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                    <div class="pr-top-row"
                      onclick={() => goToUrlDetail({preventDefault:()=>{}}, p.url)}
                      onmouseenter={(e) => { prTooltip = { x: e.clientX, y: e.clientY, url: p.url, pr: p.pagerank, depth: p.depth, intLinks: p.internal_links_out, extLinks: p.external_links_out, words: p.word_count }; }}
                      onmouseleave={() => { prTooltip = null; }}
                      style="cursor: pointer;">
                      <span class="pr-top-rank">{prTopOffset + i + 1}</span>
                      <span class="pr-top-url">{p.url.replace(/^https?:\/\/[^/]+/, '') || '/'}</span>
                      <div class="pr-top-bar-wrap">
                        <div class="pr-top-bar" style="width: {(p.pagerank / maxPR) * 100}%; opacity: {0.4 + 0.6 * (p.pagerank / maxPR)};"></div>
                      </div>
                      <span class="pr-top-score">{p.pagerank.toFixed(1)}</span>
                      <div class="pr-top-badges">
                        <span class="pr-top-badge">D{p.depth}</span>
                        <span class="pr-top-badge">{p.internal_links_out}int</span>
                      </div>
                    </div>
                  {/each}
                  {#if prTopData.total > prTopLimit}
                    <div class="pagination">
                      <button class="btn btn-sm" disabled={prTopOffset === 0} onclick={() => { prTopOffset = Math.max(0, prTopOffset - prTopLimit); loadPRSubView('top'); }}>Previous</button>
                      <span class="pagination-info">{prTopOffset + 1} - {Math.min(prTopOffset + prTopLimit, prTopData.total)} of {fmtN(prTopData.total)}</span>
                      <button class="btn btn-sm" disabled={prTopOffset + prTopLimit >= prTopData.total} onclick={() => { prTopOffset += prTopLimit; loadPRSubView('top'); }}>Next</button>
                    </div>
                  {/if}
                {:else}
                  <p class="chart-empty">No PageRank data available. Compute PageRank first.</p>
                {/if}

              {:else if prSubView === 'directory'}
                <!-- Treemap by directory -->
                {#if prTreemapData?.length > 0}
                  <div class="pr-controls">
                    <label>Depth</label>
                    <select class="pr-select" value={prTreemapDepth} onchange={(e) => { prTreemapDepth = Number(e.target.value); loadPRSubView('directory'); }}>
                      <option value={1}>1</option>
                      <option value={2}>2</option>
                      <option value={3}>3</option>
                    </select>
                    <label>Min pages</label>
                    <select class="pr-select" value={prTreemapMinPages} onchange={(e) => { prTreemapMinPages = Number(e.target.value); loadPRSubView('directory'); }}>
                      <option value={1}>1</option>
                      <option value={5}>5</option>
                      <option value={10}>10</option>
                      <option value={25}>25</option>
                    </select>
                    <span style="color: var(--text-muted); font-size: 12px;">{prTreemapData.length} directories</span>
                  </div>
                  {@const treemapItems = prTreemapData.map(d => ({ ...d, value: d.total_pr }))}
                  {@const treemapRects = squarify(treemapItems, 0, 0, 100, 100)}
                  {@const maxAvgPR = Math.max(...prTreemapData.map(d => d.avg_pr), 1)}
                  <div class="pr-treemap-container">
                    {#each treemapRects as rect}
                      {@const opacity = 0.35 + 0.65 * (rect.avg_pr / maxAvgPR)}
                      <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                      <div class="pr-treemap-rect"
                        style="left: {rect.x}%; top: {rect.y}%; width: {rect.w}%; height: {rect.h}%; background: var(--accent); opacity: {opacity};"
                        onclick={() => prDrillToTable(rect.path)}
                        onmouseenter={(e) => { prTooltip = { x: e.clientX, y: e.clientY, path: rect.path, pages: rect.page_count, totalPR: rect.total_pr, avgPR: rect.avg_pr, maxPR: rect.max_pr }; }}
                        onmouseleave={() => { prTooltip = null; }}>
                        {#if rect.w > 6 && rect.h > 5}
                          <div class="pr-treemap-label">
                            {rect.path || '/'}
                            {#if rect.w > 10 && rect.h > 8}
                              <small>{rect.page_count} pages &middot; avg {rect.avg_pr.toFixed(1)}</small>
                            {/if}
                          </div>
                        {/if}
                      </div>
                    {/each}
                  </div>
                {:else}
                  <p class="chart-empty">No PageRank data available. Compute PageRank first.</p>
                {/if}

              {:else if prSubView === 'distribution'}
                <!-- Distribution histogram -->
                {#if prDistData && prDistData.total_with_pr > 0}
                  <div class="stats-grid" style="margin-bottom: 20px;">
                    <div class="stat-card"><div class="stat-value">{fmtN(prDistData.total_with_pr)}</div><div class="stat-label">Pages with PR</div></div>
                    <div class="stat-card"><div class="stat-value">{prDistData.avg.toFixed(2)}</div><div class="stat-label">Mean</div></div>
                    <div class="stat-card"><div class="stat-value">{prDistData.median.toFixed(2)}</div><div class="stat-label">Median</div></div>
                    <div class="stat-card"><div class="stat-value">{prDistData.p90.toFixed(2)}</div><div class="stat-label">P90</div></div>
                    <div class="stat-card"><div class="stat-value">{prDistData.p99.toFixed(2)}</div><div class="stat-label">P99</div></div>
                  </div>
                  {@const distBuckets = prDistData.buckets || []}
                  {@const distMaxCount = Math.max(...distBuckets.map(b => b.count), 1)}
                  {@const histW = 600}
                  {@const histH = 300}
                  {@const histMargin = { top: 20, right: 20, bottom: 40, left: 60 }}
                  {@const plotW = histW - histMargin.left - histMargin.right}
                  {@const plotH = histH - histMargin.top - histMargin.bottom}
                  {@const barGap = 1}
                  {@const barW = distBuckets.length > 0 ? (plotW - (distBuckets.length - 1) * barGap) / distBuckets.length : 0}
                  {@const logMax = Math.log10(distMaxCount + 1)}
                  <svg viewBox="0 0 {histW} {histH}" style="width: 100%; max-width: 700px; height: auto;">
                    <!-- Y axis (log scale) -->
                    {#each [1, 10, 100, 1000, 10000, 100000] as tick}
                      {#if tick <= distMaxCount * 1.5}
                        {@const ty = histMargin.top + plotH - (logMax > 0 ? (Math.log10(tick + 1) / logMax) * plotH : 0)}
                        <line x1={histMargin.left} y1={ty} x2={histW - histMargin.right} y2={ty} stroke="var(--border)" stroke-dasharray="3,3" />
                        <text x={histMargin.left - 8} y={ty + 4} text-anchor="end" style="font-size: 10px; fill: var(--text-muted);">{tick >= 1000 ? (tick/1000) + 'k' : tick}</text>
                      {/if}
                    {/each}
                    <!-- Bars -->
                    {#each distBuckets as bucket, i}
                      {@const barH = logMax > 0 ? (Math.log10(bucket.count + 1) / logMax) * plotH : 0}
                      {@const bx = histMargin.left + i * (barW + barGap)}
                      {@const by = histMargin.top + plotH - barH}
                      {@const opacity = 0.4 + 0.6 * (bucket.count / distMaxCount)}
                      <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                      <rect class="pr-hist-bar" x={bx} y={by} width={barW} height={barH} rx="2" fill="var(--accent)" opacity={opacity}
                        onmouseenter={(e) => { prTooltip = { x: e.clientX, y: e.clientY, bucketMin: bucket.min, bucketMax: bucket.max, count: bucket.count, avgPR: bucket.avg_pr }; }}
                        onmouseleave={() => { prTooltip = null; }}
                        onclick={() => prDrillHistToTable(bucket.min, bucket.max)} />
                      {#if distBuckets.length <= 25 || i % Math.ceil(distBuckets.length / 10) === 0}
                        <text x={bx + barW / 2} y={histH - histMargin.bottom + 16} text-anchor="middle" style="font-size: 9px; fill: var(--text-muted);">{bucket.min.toFixed(0)}</text>
                      {/if}
                    {/each}
                    <!-- Axis labels -->
                    <text x={histW / 2} y={histH - 4} text-anchor="middle" style="font-size: 11px; fill: var(--text-muted);">PageRank Score</text>
                    <text x={14} y={histH / 2} text-anchor="middle" transform="rotate(-90, 14, {histH / 2})" style="font-size: 11px; fill: var(--text-muted);">Pages (log)</text>
                  </svg>
                {:else}
                  <p class="chart-empty">No PageRank data available. Compute PageRank first.</p>
                {/if}

              {:else if prSubView === 'table'}
                <!-- Full Table -->
                {#if prTableData}
                  <div class="pr-controls">
                    <label>Directory filter</label>
                    <input class="pr-dir-filter" type="text" placeholder="e.g. /blog/" bind:value={prTableDir} onkeydown={(e) => { if (e.key === 'Enter') { prTableOffset = 0; loadPRSubView('table'); } }} />
                    <button class="btn btn-sm" onclick={() => { prTableOffset = 0; loadPRSubView('table'); }}>Filter</button>
                    {#if prTableDir}
                      <button class="btn btn-sm" onclick={() => { prTableDir = ''; prTableOffset = 0; loadPRSubView('table'); }}>Clear</button>
                    {/if}
                    <span style="color: var(--text-muted); font-size: 12px;">{fmtN(prTableData.total)} pages</span>
                  </div>
                  <table>
                    <thead>
                      <tr><th>#</th><th>URL</th><th>PageRank</th><th>Depth</th><th>Int Links</th><th>Ext Links</th><th>Words</th><th>Status</th><th>Title</th></tr>
                    </thead>
                    <tbody>
                      {#each prTableData.pages || [] as p, i}
                        <tr>
                          <td style="color: var(--text-muted); font-size: 12px;">{prTableOffset + i + 1}</td>
                          <td class="cell-url"><a href={urlDetailHref(p.url)} onclick={(e) => goToUrlDetail(e, p.url)}>{p.url}</a></td>
                          <td style="color: var(--accent); font-weight: 600;">{p.pagerank.toFixed(1)}</td>
                          <td>{p.depth}</td>
                          <td>{fmtN(p.internal_links_out)}</td>
                          <td>{fmtN(p.external_links_out)}</td>
                          <td>{fmtN(p.word_count)}</td>
                          <td><span class="badge {statusBadge(p.status_code)}">{p.status_code}</span></td>
                          <td class="cell-title">{trunc(p.title, 50)}</td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                  {#if prTableData.total > 50}
                    <div class="pagination">
                      <button class="btn btn-sm" disabled={prTableOffset === 0} onclick={() => { prTableOffset = Math.max(0, prTableOffset - 50); loadPRSubView('table'); }}>Previous</button>
                      <span class="pagination-info">{prTableOffset + 1} - {Math.min(prTableOffset + 50, prTableData.total)} of {fmtN(prTableData.total)}</span>
                      <button class="btn btn-sm" disabled={prTableOffset + 50 >= prTableData.total} onclick={() => { prTableOffset += 50; loadPRSubView('table'); }}>Next</button>
                    </div>
                  {/if}
                {:else}
                  <p class="chart-empty">No PageRank data available. Compute PageRank first.</p>
                {/if}
              {/if}
            </div>

            <!-- Tooltip -->
            {#if prTooltip}
              <div class="pr-tooltip" style="left: {prTooltip.x + 12}px; top: {prTooltip.y - 10}px;">
                {#if prTooltip.url}
                  <div class="pr-tooltip-title">{prTooltip.url}</div>
                  <div class="pr-tooltip-row"><span>PageRank</span><span>{prTooltip.pr.toFixed(2)}</span></div>
                  <div class="pr-tooltip-row"><span>Depth</span><span>{prTooltip.depth}</span></div>
                  <div class="pr-tooltip-row"><span>Int links</span><span>{fmtN(prTooltip.intLinks)}</span></div>
                  <div class="pr-tooltip-row"><span>Ext links</span><span>{fmtN(prTooltip.extLinks)}</span></div>
                  <div class="pr-tooltip-row"><span>Words</span><span>{fmtN(prTooltip.words)}</span></div>
                {:else if prTooltip.path !== undefined}
                  <div class="pr-tooltip-title">{prTooltip.path || '/'}</div>
                  <div class="pr-tooltip-row"><span>Pages</span><span>{fmtN(prTooltip.pages)}</span></div>
                  <div class="pr-tooltip-row"><span>Total PR</span><span>{prTooltip.totalPR.toFixed(1)}</span></div>
                  <div class="pr-tooltip-row"><span>Avg PR</span><span>{prTooltip.avgPR.toFixed(2)}</span></div>
                  <div class="pr-tooltip-row"><span>Max PR</span><span>{prTooltip.maxPR.toFixed(2)}</span></div>
                {:else if prTooltip.bucketMin !== undefined}
                  <div class="pr-tooltip-title">PR {prTooltip.bucketMin.toFixed(1)} - {prTooltip.bucketMax.toFixed(1)}</div>
                  <div class="pr-tooltip-row"><span>Pages</span><span>{fmtN(prTooltip.count)}</span></div>
                  <div class="pr-tooltip-row"><span>Avg PR</span><span>{prTooltip.avgPR.toFixed(2)}</span></div>
                {/if}
              </div>
            {/if}

          {:else if tab === 'robots'}
            <div class="robots-layout">
              <div class="robots-hosts">
                {#if robotsLoading && robotsHosts.length === 0}
                  <p style="padding: 20px; color: var(--text-muted);">Loading...</p>
                {:else if robotsHosts.length === 0}
                  <p style="padding: 20px; color: var(--text-muted);">No robots.txt data. Run a crawl first.</p>
                {:else}
                  <table>
                    <thead>
                      <tr><th>Host</th><th>Status</th><th>Fetched</th></tr>
                    </thead>
                    <tbody>
                      {#each robotsHosts as h}
                        <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                        <tr class:robots-host-active={robotsSelectedHost === h.Host} style="cursor:pointer" onclick={() => selectRobotsHost(h.Host)}>
                          <td style="font-weight: 500;">{h.Host}</td>
                          <td><span class="badge {h.StatusCode === 200 ? 'badge-success' : h.StatusCode >= 400 ? 'badge-error' : 'badge-warning'}">{h.StatusCode}</span></td>
                          <td style="color: var(--text-muted); font-size: 12px;">{new Date(h.FetchedAt).toLocaleString()}</td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                {/if}
              </div>
              <div class="robots-detail">
                {#if robotsSelectedHost}
                  <h3 style="font-size: 14px; font-weight: 600; margin-bottom: 12px; color: var(--text-secondary);">robots.txt &mdash; {robotsSelectedHost}</h3>
                  {#if robotsContent !== null}
                    <pre class="robots-content-pre">{robotsContent || '(empty)'}</pre>
                  {:else}
                    <p style="color: var(--text-muted);">Loading...</p>
                  {/if}

                  <div class="robots-tester" style="margin-top: 20px;">
                    <h4 style="font-size: 13px; font-weight: 600; margin-bottom: 8px;">URL Tester</h4>
                    <div class="form-group" style="margin-bottom: 8px;">
                      <label>User-Agent (optional)</label>
                      <input type="text" placeholder="* (default)" bind:value={robotsTestUA} style="max-width: 300px;" />
                    </div>
                    <div class="form-group" style="margin-bottom: 8px;">
                      <label>URLs to test (one per line)</label>
                      <textarea rows="4" bind:value={robotsTestUrls} placeholder="/path/to/page&#10;/another/path" style="font-family: 'SF Mono', monospace; font-size: 13px;"></textarea>
                    </div>
                    <button class="btn btn-primary btn-sm" onclick={runRobotsTest} disabled={robotsTestLoading || !robotsTestUrls.trim()}>
                      {robotsTestLoading ? 'Testing...' : 'Test'}
                    </button>

                    {#if robotsTestResults}
                      <div style="margin-top: 12px;">
                        {#each robotsTestResults as r}
                          <div class="robots-test-result">
                            <span class="badge {r.allowed ? 'badge-success' : 'badge-error'}">{r.allowed ? 'Allowed' : 'Blocked'}</span>
                            <span style="font-size: 13px; margin-left: 8px;">{r.url}</span>
                          </div>
                        {/each}
                      </div>
                    {/if}
                  </div>
                {:else}
                  <div style="padding: 40px; text-align: center; color: var(--text-muted);">
                    <p>Select a host to view its robots.txt</p>
                  </div>
                {/if}
              </div>
            </div>

          {:else if tab === 'stats'}
            <div class="charts-container">
              {#if stats?.depth_distribution && Object.keys(stats.depth_distribution).length > 0}
                {@const depthEntries = Object.entries(stats.depth_distribution).map(([k, v]) => [Number(k), v]).sort((a, b) => a[0] - b[0])}
                {@const depthMax = Math.max(...depthEntries.map(e => e[1]))}
                {@const depthBarH = 32}
                {@const depthGap = 6}
                {@const depthSvgH = depthEntries.length * (depthBarH + depthGap)}
                <div class="chart-section">
                  <h3 class="chart-title">Depth Distribution</h3>
                  <svg class="chart-svg" viewBox="0 0 600 {depthSvgH}" preserveAspectRatio="xMinYMin meet">
                    {#each depthEntries as [depth, count], i}
                      {@const barW = depthMax > 0 ? (count / depthMax) * 440 : 0}
                      {@const y = i * (depthBarH + depthGap)}
                      {@const opacity = 0.5 + (0.5 * (1 - i / Math.max(depthEntries.length - 1, 1)))}
                      <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                      <g class="chart-bar-clickable" style="cursor:pointer" onclick={() => navigateTo(`/sessions/${selectedSession.ID}/overview`, {depth: String(depth)})}>
                        <text x="30" y={y + depthBarH / 2 + 5} text-anchor="end" class="chart-label">{depth}</text>
                        <rect x="40" y={y} width={Math.max(barW, 2)} height={depthBarH} rx="4" class="chart-bar chart-bar-accent" style="opacity: {opacity}" />
                        <text x={44 + barW} y={y + depthBarH / 2 + 5} class="chart-value">{fmtN(count)}</text>
                      </g>
                    {/each}
                  </svg>
                </div>
              {:else}
                <p class="chart-empty">No depth data available.</p>
              {/if}

              {#if stats?.status_codes && Object.keys(stats.status_codes).length > 0}
                {@const scEntries = Object.entries(stats.status_codes).map(([k, v]) => [Number(k), v]).sort((a, b) => a[0] - b[0])}
                {@const scMax = Math.max(...scEntries.map(e => e[1]))}
                {@const scBarH = 32}
                {@const scGap = 6}
                {@const scSvgH = scEntries.length * (scBarH + scGap)}
                <div class="chart-section">
                  <h3 class="chart-title">Status Code Distribution</h3>
                  <svg class="chart-svg" viewBox="0 0 600 {scSvgH}" preserveAspectRatio="xMinYMin meet">
                    {#each scEntries as [code, count], i}
                      {@const barW = scMax > 0 ? (count / scMax) * 440 : 0}
                      {@const y = i * (scBarH + scGap)}
                      {@const colorClass = code >= 200 && code < 300 ? 'chart-bar-success' : code >= 300 && code < 400 ? 'chart-bar-info' : code >= 400 && code < 500 ? 'chart-bar-warning' : 'chart-bar-error'}
                      <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
                      <g class="chart-bar-clickable" style="cursor:pointer" onclick={() => navigateTo(`/sessions/${selectedSession.ID}/overview`, {status_code: String(code)})}>
                        <text x="30" y={y + scBarH / 2 + 5} text-anchor="end" class="chart-label">{code}</text>
                        <rect x="40" y={y} width={Math.max(barW, 2)} height={scBarH} rx="4" class={`chart-bar ${colorClass}`} />
                        <text x={44 + barW} y={y + scBarH / 2 + 5} class="chart-value">{fmtN(count)}</text>
                      </g>
                    {/each}
                  </svg>
                </div>
              {:else}
                <p class="chart-empty">No status code data available.</p>
              {/if}

            </div>
          {/if}

          {#if currentData().length > 0}
            <div class="pagination">
              <button class="btn btn-sm" onclick={prevPage} disabled={currentOffset() === 0}>Previous</button>
              <span class="pagination-info">{currentOffset() + 1} - {currentOffset() + currentData().length}</span>
              <button class="btn btn-sm" onclick={nextPage} disabled={!hasMore()}>Next</button>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </main>
</div>

{#if showResumeModal}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div class="html-modal-overlay" onclick={closeResumeModal}>
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="html-modal" onclick={(e) => e.stopPropagation()} style="max-width: 480px; height: auto;">
      <div class="html-modal-header">
        <div class="html-modal-url">Resume Crawl</div>
        <div class="html-modal-actions">
          <button class="btn btn-sm" title="Close" onclick={closeResumeModal}>
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
      </div>
      <div style="padding: 20px;">
        <div class="form-grid">
          <div class="form-group"><label for="r-maxpages">Max pages (0 = unlimited)</label><input id="r-maxpages" type="number" bind:value={resumeMaxPages} min="0" /></div>
          <div class="form-group"><label for="r-maxdepth">Max depth (0 = unlimited)</label><input id="r-maxdepth" type="number" bind:value={resumeMaxDepth} min="0" /></div>
          <div class="form-group"><label for="r-workers">Workers</label><input id="r-workers" type="number" bind:value={resumeWorkers} min="1" max="100" /></div>
          <div class="form-group"><label for="r-delay">Delay</label><input id="r-delay" type="text" bind:value={resumeDelay} placeholder="1s" /></div>
          <div class="form-group" style="display: flex; flex-direction: row; align-items: center; gap: 8px; padding-top: 24px;">
            <input id="r-storehtml" type="checkbox" bind:checked={resumeStoreHtml} /><label for="r-storehtml" style="margin: 0;">Store raw HTML</label>
          </div>
        </div>
        <div style="display: flex; gap: 8px; margin-top: 20px;">
          <button class="btn btn-primary" onclick={handleResume} disabled={resuming}>
            {resuming ? 'Resuming...' : 'Resume'}
          </button>
          <button class="btn" onclick={closeResumeModal}>Cancel</button>
        </div>
      </div>
    </div>
  </div>
{/if}

{#if showHtmlModal}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div class="html-modal-overlay" onclick={closeHtmlModal}>
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="html-modal" onclick={(e) => e.stopPropagation()}>
      <div class="html-modal-header">
        <div class="html-modal-url" title={htmlModalData.url}>{htmlModalData.url}</div>
        <div class="html-modal-actions">
          <button class="btn btn-sm" class:btn-primary={htmlModalView === 'render'} onclick={() => htmlModalView = 'render'}>Render</button>
          <button class="btn btn-sm" class:btn-primary={htmlModalView === 'source'} onclick={() => htmlModalView = 'source'}>Source</button>
          <button class="btn btn-sm" title="Close" onclick={closeHtmlModal}>
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
      </div>
      <div class="html-modal-body">
        {#if htmlModalLoading}
          <p style="padding: 40px; color: var(--text-muted); text-align: center;">Loading...</p>
        {:else if !htmlModalData.body_html}
          <p style="padding: 40px; color: var(--text-muted); text-align: center;">No HTML stored for this page.</p>
        {:else if htmlModalView === 'render'}
          <iframe srcdoc={htmlModalData.body_html} title="Page render" class="html-modal-iframe"></iframe>
        {:else}
          <pre class="html-modal-source"><code>{htmlModalData.body_html}</code></pre>
        {/if}
      </div>
    </div>
  </div>
{/if}
