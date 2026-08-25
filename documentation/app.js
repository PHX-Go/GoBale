/**
 * GoBale Documentation Engine
 * Features: Deep Full-Text Search Engine (Index over all HTML content), Dynamic ScrollSpy, Clipboard Copy, Mobile Drawer
 * Strictly Zero Emojis
 */

var searchIndex = [];

document.addEventListener('DOMContentLoaded', function() {
  buildSearchIndex();
  initSearch();
  initCopyCode();
  initScrollSpy();
  initMobileDrawer();
  initSidebarLinks();
});

// Build dynamic full-text search index from the real DOM content
function buildSearchIndex() {
  searchIndex = [];
  var chapters = document.querySelectorAll('.doc-chapter');

  chapters.forEach(function(chapter) {
    var chapterId = chapter.getAttribute('id');
    var chapterHeader = chapter.querySelector('.chapter-title');
    var chapterTitleText = chapterHeader ? chapterHeader.innerText.trim() : chapterId;

    // Index the entire chapter
    var chapterLead = chapter.querySelector('.chapter-lead');
    var leadText = chapterLead ? chapterLead.innerText.trim() : '';

    // Index all sub-sections within chapter
    var sections = chapter.querySelectorAll('.doc-section');
    if (sections.length > 0) {
      sections.forEach(function(section) {
        var sectionTitle = section.querySelector('.section-title, .subsection-title');
        var sTitleText = sectionTitle ? sectionTitle.innerText.trim() : chapterTitleText;

        // Collect all text from paragraphs, code snippets, tables, callouts in this section
        var paragraphs = section.querySelectorAll('p, code, tr, .callout-content');
        var fullTextParts = [];
        paragraphs.forEach(function(p) {
          fullTextParts.push(p.innerText.trim());
        });
        var sectionBody = fullTextParts.join(' ');

        searchIndex.push({
          targetId: chapterId,
          sectionTitle: sTitleText,
          chapterTitle: chapterTitleText,
          content: sectionBody,
          keywords: (sTitleText + ' ' + chapterTitleText + ' ' + sectionBody).toLowerCase()
        });
      });
    } else {
      searchIndex.push({
        targetId: chapterId,
        sectionTitle: chapterTitleText,
        chapterTitle: chapterTitleText,
        content: leadText,
        keywords: (chapterTitleText + ' ' + leadText).toLowerCase()
      });
    }
  });

  console.log('GoBale Search Index ready with ' + searchIndex.length + ' indexed searchable sections.');
}

function initSearch() {
  var triggerBtn = document.getElementById('searchTriggerBtn');
  var modalBackdrop = document.getElementById('searchModalBackdrop');
  var searchInput = document.getElementById('searchInput');
  var resultsList = document.getElementById('searchResultsList');

  if (!triggerBtn || !modalBackdrop || !searchInput || !resultsList) return;

  function openSearch() {
    modalBackdrop.classList.add('open');
    searchInput.value = '';
    renderSearchResults('');
    setTimeout(function() { searchInput.focus(); }, 60);
  }

  function closeSearch() {
    modalBackdrop.classList.remove('open');
  }

  triggerBtn.addEventListener('click', function(e) {
    e.preventDefault();
    openSearch();
  });

  modalBackdrop.addEventListener('click', function(e) {
    if (e.target === modalBackdrop) closeSearch();
  });

  document.addEventListener('keydown', function(e) {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      modalBackdrop.classList.contains('open') ? closeSearch() : openSearch();
    }
    if (e.key === 'Escape' && modalBackdrop.classList.contains('open')) {
      closeSearch();
    }
  });

  searchInput.addEventListener('input', function(e) {
    var query = e.target.value.trim().toLowerCase();
    renderSearchResults(query);
  });

  function renderSearchResults(query) {
    resultsList.innerHTML = '';

    if (!query) {
      // Show first 6 main chapters as quick suggestions
      var suggestions = searchIndex.slice(0, 7);
      suggestions.forEach(function(item) {
        resultsList.appendChild(createResultItem(item, ''));
      });
      return;
    }

    var terms = query.split(' ').filter(function(t) { return t.length > 0; });
    var matches = [];

    searchIndex.forEach(function(item) {
      var score = 0;
      var matchPos = -1;

      terms.forEach(function(term) {
        if (item.sectionTitle.toLowerCase().indexOf(term) !== -1) {
          score += 10;
        }
        if (item.chapterTitle.toLowerCase().indexOf(term) !== -1) {
          score += 5;
        }
        var pos = item.content.toLowerCase().indexOf(term);
        if (pos !== -1) {
          score += 2;
          if (matchPos === -1) matchPos = pos;
        }
      });

      if (score > 0) {
        matches.push({ item: item, score: score, matchPos: matchPos });
      }
    });

    matches.sort(function(a, b) { return b.score - a.score; });

    if (matches.length === 0) {
      resultsList.innerHTML = '<li style="padding: 2rem; text-align: center; color: var(--text-muted); font-size: 0.92rem;">عبارت مورد نظر در متن داکیومنت یافت نشد.</li>';
      return;
    }

    matches.slice(0, 15).forEach(function(m) {
      resultsList.appendChild(createResultItem(m.item, query, m.matchPos));
    });
  }

  function createResultItem(item, query, matchPos) {
    var li = document.createElement('li');
    li.className = 'search-result-item';

    // Generate highlighted snippet around matched text
    var snippet = '';
    if (query && matchPos !== undefined && matchPos >= 0) {
      var start = Math.max(0, matchPos - 45);
      var end = Math.min(item.content.length, matchPos + 90);
      var rawSnippet = item.content.substring(start, end);
      if (start > 0) rawSnippet = '...' + rawSnippet;
      if (end < item.content.length) rawSnippet = rawSnippet + '...';

      // Highlight match
      var regex = new RegExp('(' + escapeRegExp(query) + ')', 'gi');
      snippet = escapeHtml(rawSnippet).replace(regex, '<mark class="search-highlight">$1</mark>');
    } else {
      snippet = escapeHtml(item.content.substring(0, 110)) + (item.content.length > 110 ? '...' : '');
    }

    li.innerHTML = 
      '<div class="search-result-header">' +
        '<div class="search-result-title">' +
          '<svg class="icon icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="16" y1="13" x2="8" y2="13"></line><line x1="16" y1="17" x2="8" y2="17"></line><polyline points="10 9 9 9 8 9"></polyline></svg>' +
          '<span>' + escapeHtml(item.sectionTitle) + '</span>' +
        '</div>' +
        '<span class="search-result-category">' + escapeHtml(item.chapterTitle.split(':')[0]) + '</span>' +
      '</div>' +
      '<div class="search-result-snippet">' + snippet + '</div>';

    li.addEventListener('click', function() {
      closeSearch();
      var targetEl = document.getElementById(item.targetId);
      if (targetEl) {
        targetEl.scrollIntoView({ behavior: 'smooth' });
        updateActiveSidebar(item.targetId);
      }
    });

    return li;
  }
}

function escapeHtml(text) {
  var div = document.createElement('div');
  div.innerText = text;
  return div.innerHTML;
}

function escapeRegExp(string) {
  return string.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function initCopyCode() {
  document.querySelectorAll('.code-copy-btn').forEach(function(btn) {
    btn.addEventListener('click', function() {
      var container = btn.closest('.code-container');
      if (!container) return;
      var pre = container.querySelector('pre code');
      if (!pre) return;

      var textToCopy = pre.innerText;
      navigator.clipboard.writeText(textToCopy).then(function() {
        var originalHtml = btn.innerHTML;
        btn.classList.add('copied');
        btn.innerHTML = '<svg class="icon icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor"><polyline points="20 6 9 17 4 12"></polyline></svg><span>کپی شد</span>';
        showToast('کد با موفقیت کپی شد');

        setTimeout(function() {
          btn.classList.remove('copied');
          btn.innerHTML = originalHtml;
        }, 2000);
      }).catch(function() {
        showToast('خطا در کپی در کلیپ‌بورد');
      });
    });
  });
}

function showToast(message) {
  var container = document.querySelector('.toast-container');
  if (!container) {
    container = document.createElement('div');
    container.className = 'toast-container';
    document.body.appendChild(container);
  }

  var toast = document.createElement('div');
  toast.className = 'toast';
  toast.innerHTML = '<svg class="icon icon-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path><polyline points="22 4 12 14.01 9 11.01"></polyline></svg><span>' + message + '</span>';

  container.appendChild(toast);
  setTimeout(function() {
    toast.style.opacity = '0';
    toast.style.transform = 'translateY(10px) scale(0.9)';
    toast.style.transition = 'all 0.25s ease';
    setTimeout(function() { toast.remove(); }, 250);
  }, 2500);
}

function updateActiveSidebar(activeId) {
  if (!activeId) return;

  var sidebarLinks = document.querySelectorAll('.sidebar-link');
  sidebarLinks.forEach(function(link) {
    var href = link.getAttribute('href');
    if (href === '#' + activeId) {
      link.classList.add('active');
    } else {
      link.classList.remove('active');
    }
  });
}

function initSidebarLinks() {
  document.querySelectorAll('.sidebar-link').forEach(function(link) {
    link.addEventListener('click', function(e) {
      var href = link.getAttribute('href');
      if (href && href.startsWith('#')) {
        var targetId = href.substring(1);
        var targetEl = document.getElementById(targetId);
        if (targetEl) {
          e.preventDefault();
          targetEl.scrollIntoView({ behavior: 'smooth' });
          updateActiveSidebar(targetId);
          if (history.pushState) {
            history.pushState(null, null, '#' + targetId);
          }
        }
      }
    });
  });
}

function initScrollSpy() {
  var chapters = document.querySelectorAll('.doc-chapter');
  if (chapters.length === 0) return;

  function onScroll() {
    var scrollPos = window.scrollY || window.pageYOffset;
    var headerOffset = 140;
    var currentChapterId = null;

    chapters.forEach(function(ch) {
      var top = ch.offsetTop - headerOffset;
      var bottom = top + ch.offsetHeight;
      if (scrollPos >= top && scrollPos < bottom) {
        currentChapterId = ch.getAttribute('id');
      }
    });

    if (!currentChapterId && scrollPos < (chapters[0].offsetTop - headerOffset)) {
      currentChapterId = chapters[0].getAttribute('id');
    } else if (!currentChapterId && chapters.length > 0) {
      currentChapterId = chapters[chapters.length - 1].getAttribute('id');
    }

    if (currentChapterId) {
      updateActiveSidebar(currentChapterId);
    }
  }

  window.addEventListener('scroll', onScroll, { passive: true });
  onScroll();
}

function initMobileDrawer() {
  var toggleBtn = document.getElementById('menuToggleBtn');
  var sidebar = document.getElementById('sidebar');

  if (!toggleBtn || !sidebar) return;

  toggleBtn.addEventListener('click', function(e) {
    e.stopPropagation();
    sidebar.classList.toggle('open');
  });

  document.addEventListener('click', function(e) {
    if (window.innerWidth <= 1024) {
      if (!sidebar.contains(e.target) && !toggleBtn.contains(e.target) && sidebar.classList.contains('open')) {
        sidebar.classList.remove('open');
      }
    }
  });

  sidebar.querySelectorAll('.sidebar-link').forEach(function(link) {
    link.addEventListener('click', function() {
      if (window.innerWidth <= 1024) {
        sidebar.classList.remove('open');
      }
    });
  });
}
