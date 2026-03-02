<script>
  let {
    options = [],
    value = $bindable(),
    placeholder = '',
    disabled = false,
    small = false,
    id = undefined,
    onsearch = undefined,
    onchange = undefined,
  } = $props();

  let open = $state(false);
  let query = $state('');
  let activeIndex = $state(-1);
  let triggerEl = $state(null);
  let dropdownEl = $state(null);
  let inputEl = $state(null);
  let flipUp = $state(false);
  let dropStyle = $state('');
  let asyncOptions = $state([]);
  let asyncLoading = $state(false);
  let debounceTimer = null;

  let displayOptions = $derived(
    onsearch
      ? query
        ? asyncOptions
        : options
      : query
        ? options.filter((o) => o.label.toLowerCase().includes(query.toLowerCase()))
        : options,
  );

  let selectedLabel = $derived(
    options.find((o) => String(o.value) === String(value))?.label ||
      (onsearch && asyncOptions.find((o) => String(o.value) === String(value))?.label) ||
      '',
  );

  function openDropdown() {
    if (disabled) return;
    if (triggerEl) {
      const rect = triggerEl.getBoundingClientRect();
      dropStyle = `position:fixed;left:${rect.left}px;top:${rect.bottom + 4}px;width:${rect.width}px;`;
    }
    open = true;
    query = '';
    activeIndex = -1;
    asyncOptions = [];
    requestAnimationFrame(() => {
      positionDropdown();
      inputEl?.focus();
    });
  }

  function closeDropdown() {
    open = false;
    query = '';
    activeIndex = -1;
    clearTimeout(debounceTimer);
    triggerEl?.focus();
  }

  function positionDropdown() {
    if (!triggerEl || !dropdownEl) return;
    const rect = triggerEl.getBoundingClientRect();
    const spaceBelow = window.innerHeight - rect.bottom;
    const dropH = Math.min(dropdownEl.scrollHeight, 280);
    flipUp = spaceBelow < dropH + 8 && rect.top > dropH + 8;
    if (flipUp) {
      dropStyle = `position:fixed;left:${rect.left}px;bottom:${window.innerHeight - rect.top + 4}px;width:${rect.width}px;`;
    } else {
      dropStyle = `position:fixed;left:${rect.left}px;top:${rect.bottom + 4}px;width:${rect.width}px;`;
    }
  }

  function selectOption(opt) {
    value = opt.value;
    onchange?.(opt.value);
    closeDropdown();
  }

  function handleTriggerKeydown(e) {
    if (e.key === 'Enter' || e.key === ' ' || e.key === 'ArrowDown') {
      e.preventDefault();
      openDropdown();
    }
  }

  function handleInputKeydown(e) {
    if (e.key === 'Escape') {
      e.preventDefault();
      closeDropdown();
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      activeIndex = Math.min(activeIndex + 1, displayOptions.length - 1);
      scrollActiveIntoView();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      activeIndex = Math.max(activeIndex - 1, 0);
      scrollActiveIntoView();
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (activeIndex >= 0 && activeIndex < displayOptions.length) {
        selectOption(displayOptions[activeIndex]);
      }
    } else if (e.key === 'Tab') {
      closeDropdown();
    }
  }

  function scrollActiveIntoView() {
    requestAnimationFrame(() => {
      const el = dropdownEl?.querySelector(`[data-index="${activeIndex}"]`);
      el?.scrollIntoView({ block: 'nearest' });
    });
  }

  function handleClickOutside(e) {
    if (
      open &&
      triggerEl &&
      !triggerEl.contains(e.target) &&
      dropdownEl &&
      !dropdownEl.contains(e.target)
    ) {
      closeDropdown();
    }
  }

  function handleInput() {
    activeIndex = -1;
    if (onsearch && query) {
      clearTimeout(debounceTimer);
      asyncLoading = true;
      debounceTimer = setTimeout(async () => {
        try {
          asyncOptions = await onsearch(query);
        } catch {
          asyncOptions = [];
        } finally {
          asyncLoading = false;
        }
      }, 300);
    }
  }

  $effect(() => {
    if (open) {
      document.addEventListener('mousedown', handleClickOutside);
      window.addEventListener('scroll', positionDropdown, true);
      window.addEventListener('resize', positionDropdown);
      return () => {
        document.removeEventListener('mousedown', handleClickOutside);
        window.removeEventListener('scroll', positionDropdown, true);
        window.removeEventListener('resize', positionDropdown);
      };
    }
  });
</script>

<div class="ss-wrap" class:ss-small={small}>
  <button
    bind:this={triggerEl}
    {id}
    type="button"
    class="ss-trigger"
    class:ss-open={open}
    class:ss-disabled={disabled}
    {disabled}
    role="combobox"
    aria-expanded={open}
    aria-haspopup="listbox"
    onclick={openDropdown}
    onkeydown={handleTriggerKeydown}
  >
    <span class="ss-trigger-label" class:ss-placeholder={!selectedLabel}>
      {selectedLabel || placeholder || '\u00A0'}
    </span>
    <svg class="ss-chevron" viewBox="0 0 20 20" fill="currentColor" width="16" height="16">
      <path
        fill-rule="evenodd"
        d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z"
        clip-rule="evenodd"
      />
    </svg>
  </button>

  {#if open}
    <div
      bind:this={dropdownEl}
      class="ss-dropdown"
      class:ss-flip={flipUp}
      style={dropStyle}
      role="listbox"
    >
      <div class="ss-search-wrap">
        <input
          bind:this={inputEl}
          class="ss-search"
          type="text"
          bind:value={query}
          oninput={handleInput}
          onkeydown={handleInputKeydown}
          placeholder="..."
          autocomplete="off"
          spellcheck="false"
        />
      </div>
      <div class="ss-options">
        {#if asyncLoading}
          <div class="ss-empty">...</div>
        {:else if displayOptions.length === 0}
          <div class="ss-empty">&mdash;</div>
        {:else}
          {#each displayOptions as opt, i}
            <div
              class="ss-option"
              class:ss-active={i === activeIndex}
              class:ss-selected={String(opt.value) === String(value)}
              role="option"
              aria-selected={String(opt.value) === String(value)}
              data-index={i}
              onmouseenter={() => (activeIndex = i)}
              onmousedown={(e) => {
                e.preventDefault();
                selectOption(opt);
              }}
            >
              {opt.label}
            </div>
          {/each}
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .ss-wrap {
    position: relative;
    display: inline-flex;
    width: 100%;
  }

  .ss-trigger {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    padding: 9px 14px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-input);
    color: var(--text);
    font-size: 14px;
    font-family: inherit;
    cursor: pointer;
    transition: border-color 0.15s;
    gap: 8px;
    text-align: left;
    line-height: 1.4;
  }

  .ss-trigger:hover:not(.ss-disabled) {
    border-color: var(--text-muted);
  }

  .ss-trigger:focus {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-light);
  }

  .ss-trigger.ss-open {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-light);
  }

  .ss-trigger.ss-disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .ss-trigger-label {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }

  .ss-placeholder {
    color: var(--text-muted);
  }

  .ss-chevron {
    flex-shrink: 0;
    color: var(--text-muted);
    transition: transform 0.15s;
  }

  .ss-open .ss-chevron {
    transform: rotate(180deg);
  }

  .ss-dropdown {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    box-shadow: var(--shadow-md);
    z-index: 9999;
    overflow: hidden;
  }

  .ss-search-wrap {
    padding: 6px;
    border-bottom: 1px solid var(--border-light);
  }

  .ss-search {
    width: 100%;
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg-input);
    color: var(--text);
    font-size: 13px;
    font-family: inherit;
    outline: none;
  }

  .ss-search:focus {
    border-color: var(--accent);
  }

  .ss-options {
    max-height: 220px;
    overflow-y: auto;
    padding: 4px 0;
  }

  .ss-option {
    padding: 7px 12px;
    font-size: 13px;
    cursor: pointer;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    transition: background 0.05s;
  }

  .ss-option.ss-active {
    background: var(--bg-hover);
  }

  .ss-option.ss-selected {
    color: var(--accent);
    font-weight: 600;
  }

  .ss-empty {
    padding: 12px;
    text-align: center;
    color: var(--text-muted);
    font-size: 13px;
  }

  /* Small variant */
  .ss-small .ss-trigger {
    padding: 6px 10px;
    font-size: 13px;
  }

  .ss-small .ss-chevron {
    width: 14px;
    height: 14px;
  }
</style>
