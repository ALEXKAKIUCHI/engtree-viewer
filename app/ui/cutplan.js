// Cut plan tab: product and quantity, the conference table (corrections
// for this plan only), the material list, the machine values, the run and
// the drawings. Loaded after the main script; uses its t(), esc(), toast()
// and the I18N table.

Object.assign(I18N.en, {
  cut_tab: 'Cut plan',
  s_cut_h: 'Cut plan columns',
  s_cut_hint: 'Where the cut plan finds its data in the BOM (or the details sheet). Same rules as above. Quantity comes from the Quantity column.',
  s_cut_reports: 'Folder for cut plan reports (optional)', s_cut_reports_hint: 'Each PDF report and summary spreadsheet is also saved here, for shops that do not print.',
  cut_xlsx: 'Summary (.xlsx)', cut_save_error: 'Could not save in the reports folder: {0}',
  s_cut_material: 'Material code (e.g. Cód. MP)', s_cut_length: 'Length', s_cut_width: 'Width', s_cut_unit: 'Unit of the sizes',
  cut_product: 'Product', cut_product_ph: 'product code', cut_qty: 'Quantity', cut_load: 'Load parts',
  cut_add_line: '+ Line', cut_materials_btn: 'Material list…', cut_none: 'Choose a product and click “Load parts”, or add lines by hand.',
  cut_c_use: 'Use', cut_c_item: 'Item', cut_c_code: 'Code', cut_c_desc: 'Description', cut_c_mat: 'Material',
  cut_c_kind: 'Type', cut_c_len: 'Length', cut_c_wid: 'Width', cut_c_qty: 'Qty', cut_c_rot: 'Turns', cut_c_warn: 'Warnings',
  cut_swap: 'Swap length and width', cut_register: 'register',
  kind_sheet: 'sheet', kind_bar: 'bar',
  w_no_length: 'no length', w_no_width: 'no width', w_bad_qty: 'check quantity', w_unknown_material: 'material not in the list',
  w_too_big: 'larger than the sheet/bar', w_no_material_column: 'The BOM has no material column (see Settings › Cut plan columns).',
  w_no_length_column: 'The BOM has no length column.', w_no_width_column: 'The BOM has no width column.',
  cut_machine: 'Machine', cut_saw: 'Panel saw', cut_bars: 'Bars and tubes',
  cut_saw_kerf: 'Saw kerf (mm)', cut_edge_trim: 'Edge trim (mm)', cut_stages: 'Stages', cut_first: 'First cut',
  cut_first_auto: 'best', cut_first_length: 'along the length', cut_first_width: 'along the width',
  cut_bar_kerf: 'Kerf (mm)', cut_start_loss: 'Loss at start (mm)', cut_end_loss: 'Loss at end (mm)',
  cut_time: 'Time per material (s)', cut_order: 'Order', cut_user: 'User',
  cut_choose: 'Materials to calculate', cut_run: 'Calculate', cut_stop: 'Stop', cut_pdf: 'PDF report',
  cut_state_waiting: 'waiting', cut_state_running: 'calculating…', cut_state_done: 'done', cut_state_error: 'error', cut_state_stopped: 'stopped',
  cut_verified: 'verified', cut_refused: 'verifier refused',
  cut_sheets: '{0} sheet(s)', cut_bars_n: '{0} bar(s)', cut_util: 'use {0}%', cut_plan_n: 'Plan {0}/{1}', cut_times: '× {0}',
  cut_unplaced: 'Not placed: {0}', cut_left_out: 'Left out',
  cut_offcut: 'offcut', cut_bar: 'Bar',
  lp_unknown_material: 'material {0} is not in the list', lp_no_qty: 'no quantity', lp_no_size: 'no size to cut', cut_scrap: 'scrap',
  m_title: 'Material list', m_hint: 'One list for every project, kept on this computer. The code must be the one in the BOM material column.',
  m_code: 'Code', m_alt: 'Alt. code', m_desc: 'Description', m_kind: 'Type', m_thick: 'Thick.', m_grain: 'Grain',
  m_len: 'Length', m_wid: 'Width', m_min: 'Min. offcut L×W', m_offcuts: 'Offcuts in stock', m_offcuts_ph: '1000x800x2; 600x500',
  m_default_min: 'Smallest useful offcut (general)', m_add: '+ Material', m_save: 'Save', m_saved: 'Material list saved',
  m_import: 'Import spreadsheet…', m_export_xlsx: 'Export .xlsx', m_export_csv: 'Export .csv', m_template: 'Blank template',
  m_io_hint: 'Export takes the saved list. An import fills this screen; check it and click Save.',
  m_imported: '{0} new, {1} updated. Check and click Save.', m_import_missing: 'Columns not found: {0}. The sheet needs at least Code and Length.',
  m_import_rows: 'Left out:', m_row: 'row {0}',
  mi_no_code: 'no code', mi_bad_type: 'type "{0}" is neither sheet nor bar', mi_bad_grain: 'grain "{0}" is neither yes nor no',
  mi_not_number: '"{0}" is not a number', mi_bad_offcut: 'offcuts "{0}" are not written as length x width x quantity',
  mi_no_size: 'size missing (sheet: length and width; bar: length)', mi_negative_thickness: 'negative thickness',
  mi_repeated_code: 'code used by two materials ({0})'
});
Object.assign(I18N['pt-BR'], {
  cut_tab: 'Plano de corte',
  s_cut_h: 'Colunas do plano de corte',
  s_cut_hint: 'Onde o plano de corte acha os dados na BOM (ou na planilha de detalhes). Mesmas regras de cima. A quantidade vem da coluna Quantidade.',
  s_cut_reports: 'Pasta dos relatórios do plano de corte (opcional)', s_cut_reports_hint: 'Cada relatório PDF e planilha de resumo também é gravado aqui, para quem não trabalha com papel.',
  cut_xlsx: 'Resumo (.xlsx)', cut_save_error: 'Não foi possível gravar na pasta dos relatórios: {0}',
  s_cut_material: 'Código do material (ex.: Cód. MP)', s_cut_length: 'Comprimento', s_cut_width: 'Largura', s_cut_unit: 'Unidade das medidas',
  cut_product: 'Produto', cut_product_ph: 'código do produto', cut_qty: 'Quantidade', cut_load: 'Carregar peças',
  cut_add_line: '+ Linha', cut_materials_btn: 'Cadastro de materiais…', cut_none: 'Escolha um produto e clique em “Carregar peças”, ou acrescente linhas à mão.',
  cut_c_use: 'Usar', cut_c_item: 'Item', cut_c_code: 'Código', cut_c_desc: 'Descrição', cut_c_mat: 'Material',
  cut_c_kind: 'Tipo', cut_c_len: 'Comp.', cut_c_wid: 'Larg.', cut_c_qty: 'Qtd', cut_c_rot: 'Gira', cut_c_warn: 'Avisos',
  cut_swap: 'Inverter comprimento e largura', cut_register: 'cadastrar',
  kind_sheet: 'chapa', kind_bar: 'barra',
  w_no_length: 'sem comprimento', w_no_width: 'sem largura', w_bad_qty: 'conferir quantidade', w_unknown_material: 'material não cadastrado',
  w_too_big: 'maior que a chapa/barra', w_no_material_column: 'A BOM não tem coluna de material (veja Configurações › Colunas do plano de corte).',
  w_no_length_column: 'A BOM não tem coluna de comprimento.', w_no_width_column: 'A BOM não tem coluna de largura.',
  cut_machine: 'Máquina', cut_saw: 'Seccionadora', cut_bars: 'Barras e tubos',
  cut_saw_kerf: 'Espessura da serra (mm)', cut_edge_trim: 'Refilo (mm)', cut_stages: 'Etapas', cut_first: 'Primeiro corte',
  cut_first_auto: 'o melhor', cut_first_length: 'pelo comprimento', cut_first_width: 'pela largura',
  cut_bar_kerf: 'Espessura do corte (mm)', cut_start_loss: 'Perda no início (mm)', cut_end_loss: 'Perda no fim (mm)',
  cut_time: 'Tempo por material (s)', cut_order: 'Pedido', cut_user: 'Usuário',
  cut_choose: 'Materiais a calcular', cut_run: 'Calcular', cut_stop: 'Parar', cut_pdf: 'Relatório PDF',
  cut_state_waiting: 'na fila', cut_state_running: 'calculando…', cut_state_done: 'pronto', cut_state_error: 'erro', cut_state_stopped: 'parado',
  cut_verified: 'conferido', cut_refused: 'recusado pelo verificador',
  cut_sheets: '{0} chapa(s)', cut_bars_n: '{0} barra(s)', cut_util: 'aproveitamento {0}%', cut_plan_n: 'Plano {0}/{1}', cut_times: '× {0}',
  cut_unplaced: 'Não couberam: {0}', cut_left_out: 'Ficaram de fora',
  cut_offcut: 'retalho', cut_bar: 'Barra',
  lp_unknown_material: 'material {0} não cadastrado', lp_no_qty: 'sem quantidade', lp_no_size: 'sem medida para cortar', cut_scrap: 'descarte',
  m_title: 'Cadastro de materiais', m_hint: 'Um cadastro para todos os projetos, guardado neste computador. O código tem que ser o mesmo da coluna de material da BOM.',
  m_code: 'Código', m_alt: 'Cód. alternativo', m_desc: 'Descrição', m_kind: 'Tipo', m_thick: 'Esp.', m_grain: 'Veio',
  m_len: 'Comp.', m_wid: 'Larg.', m_min: 'Sobra mín. C×L', m_offcuts: 'Retalhos em estoque', m_offcuts_ph: '1000x800x2; 600x500',
  m_default_min: 'Menor sobra aproveitável (geral)', m_add: '+ Material', m_save: 'Gravar', m_saved: 'Cadastro gravado',
  m_import: 'Importar planilha…', m_export_xlsx: 'Exportar .xlsx', m_export_csv: 'Exportar .csv', m_template: 'Modelo em branco',
  m_io_hint: 'A exportação leva o cadastro gravado. A importação preenche esta tela; confira e clique em Gravar.',
  m_imported: '{0} novos, {1} atualizados. Confira e clique em Gravar.', m_import_missing: 'Colunas não encontradas: {0}. A planilha precisa ter pelo menos Código e Comprimento.',
  m_import_rows: 'Ficaram de fora:', m_row: 'linha {0}',
  mi_no_code: 'sem código', mi_bad_type: 'tipo "{0}" não é chapa nem barra', mi_bad_grain: 'veio "{0}" não é sim nem não',
  mi_not_number: '"{0}" não é um número', mi_bad_offcut: 'retalhos "{0}" não estão como comprimento x largura x quantidade',
  mi_no_size: 'falta medida (chapa: comprimento e largura; barra: comprimento)', mi_negative_thickness: 'espessura negativa',
  mi_repeated_code: 'código usado por dois materiais ({0})'
});

// fileNameOf reads the file name from a Content-Disposition header, in the
// UTF-8 form (filename*=utf-8''...) or the plain one.
function fileNameOf(r, fallback) {
  const h = r.headers.get('Content-Disposition') || '';
  const star = h.match(/filename\*=utf-8''([^;]+)/i);
  if (star) try { return decodeURIComponent(star[1]); } catch (e) {}
  return h.match(/filename="?([^";]+)"?/)?.[1] || fallback;
}

const cut = { ready: false, lines: [], mats: { materials: [] }, machine: null, status: null, poll: null, product: '', productDesc: '' };

function num(v) { const n = parseFloat(String(v ?? '').replace(',', '.')); return isFinite(n) ? n : 0; }
function fmtN(v) { return (Math.round(v * 10) / 10).toLocaleString(lang); }
function findMat(code) {
  const k = String(code || '').trim().toUpperCase();
  if (!k) return null;
  return cut.mats.materials.find(m => m.code.toUpperCase() === k || (m.alt_code || '').toUpperCase() === k) || null;
}

async function cutOpen() {
  if (cut.ready) return;
  cut.ready = true;
  const pane = document.getElementById('pane-cut');
  pane.innerHTML = `
  <style>
    #pane-cut { overflow: auto; }
    .cut { padding: 10px 14px; display: flex; flex-direction: column; gap: 10px; width: 100%; }
    .cut > * { flex-shrink: 0; }
    .cut .card { background: #232336; border: 1px solid #3a3a4e; border-radius: 6px; padding: 10px 12px; }
    .cut h3 { margin: 0 0 8px; font-size: 10.5pt; color: #93c5fd; }
    .cut table.conf td { padding: 2px 4px; }
    .cut table.conf input { padding: 3px 5px; width: 100%; }
    .cut table.conf input.n { width: 70px; text-align: right; }
    .cut table.conf input.b { width: 44px; text-align: right; }
    .cut table.conf tr.off td { opacity: .4; }
    .cut table.conf tbody tr:hover { cursor: default; }
    .cut .w { color: #fde68a; font-size: 9pt; }
    .cut .mgrid { display: grid; grid-template-columns: repeat(auto-fit, minmax(170px, 1fr)); gap: 6px 14px; }
    .cut .mgrid label { display: block; font-size: 9pt; color: #cfd3e0; margin-bottom: 2px; }
    .cut .mgrid input, .cut .mgrid select { width: 100%; }
    .cut .res-item { margin: 10px 0; }
    .cut .res-item svg { width: 100%; max-height: 560px; background: #fff; border-radius: 4px; display: block; }
    .cut .ok { color: #34d399; } .cut .bad { color: #f87171; }
    .cut .bars { font-family: Consolas, monospace; font-size: 9.5pt; line-height: 1.6; }
    #mat-modal table td { padding: 2px 3px; } #mat-modal input, #mat-modal select { padding: 3px 5px; width: 100%; }
  </style>
  <div class="cut">
    <div class="row" style="flex-wrap: wrap">
      <label data-i18n="cut_product"></label>
      <input id="cut-product" list="cut-products" style="width: 200px" data-i18n-ph="cut_product_ph">
      <datalist id="cut-products"></datalist>
      <label data-i18n="cut_qty"></label><input id="cut-qty" type="number" min="1" value="1" style="width: 70px">
      <button class="primary" onclick="cutLoad()" data-i18n="cut_load"></button>
      <button onclick="cutAddLine()" data-i18n="cut_add_line"></button>
      <span class="grow"></span>
      <button onclick="matOpen()" data-i18n="cut_materials_btn"></button>
    </div>
    <div id="cut-warn"></div>
    <div class="card" style="overflow-x: auto"><div id="cut-table"></div></div>
    <div class="card">
      <h3 data-i18n="cut_machine"></h3>
      <div class="mgrid" id="cut-machine"></div>
    </div>
    <div class="card">
      <h3 data-i18n="cut_choose"></h3>
      <div class="row" style="flex-wrap: wrap" id="cut-choose"></div>
      <div class="row" style="margin-top: 10px">
        <button class="primary" id="cut-run" onclick="cutRun()" data-i18n="cut_run"></button>
        <button id="cut-stop" onclick="cutStop()" data-i18n="cut_stop" disabled></button>
        <button id="cut-pdf" onclick="cutPDF()" data-i18n="cut_pdf" disabled></button>
        <button id="cut-xlsx" onclick="cutXLSX()" data-i18n="cut_xlsx" disabled></button>
        <span class="muted" id="cut-run-state"></span>
      </div>
    </div>
    <div id="cut-result"></div>
  </div>
  <div class="modal-bg" id="mat-modal"><div class="modal" style="width: 1180px">
    <h2 data-i18n="m_title"></h2>
    <div class="muted" data-i18n="m_hint"></div>
    <div class="row" style="margin: 8px 0"><label data-i18n="m_default_min"></label>
      <input id="m-min-l" style="width: 80px"> × <input id="m-min-w" style="width: 80px"> mm</div>
    <div style="max-height: 55vh; overflow: auto"><table><thead><tr>
      <th data-i18n="m_code"></th><th data-i18n="m_alt"></th><th data-i18n="m_desc"></th><th data-i18n="m_kind"></th>
      <th data-i18n="m_thick"></th><th data-i18n="m_grain"></th><th data-i18n="m_len"></th><th data-i18n="m_wid"></th>
      <th data-i18n="m_min"></th><th data-i18n="m_offcuts"></th><th></th></tr></thead><tbody id="m-body"></tbody></table></div>
    <div id="m-error"></div>
    <div class="row" style="margin-top: 10px; flex-wrap: wrap">
      <button onclick="document.getElementById('m-file').click()" data-i18n="m_import"></button>
      <input type="file" id="m-file" accept=".xlsx,.csv,.txt" style="display: none" onchange="matImport(this)">
      <button onclick="matExport('xlsx')" data-i18n="m_export_xlsx"></button>
      <button onclick="matExport('csv')" data-i18n="m_export_csv"></button>
      <button onclick="matExport('xlsx', true)" data-i18n="m_template"></button>
      <span class="muted" data-i18n="m_io_hint"></span>
    </div>
    <div class="modal-actions"><button onclick="matAddRow()" data-i18n="m_add"></button><span class="grow"></span>
      <button onclick="closeModal('mat-modal')" data-i18n="close"></button>
      <button class="primary" onclick="matSave()" data-i18n="m_save"></button></div>
  </div></div>`;
  applyI18n(pane);
  const [mats, machine, products, status] = await Promise.all([
    fetch('/api/materials').then(r => r.json()),
    fetch('/api/cut_machine').then(r => r.json()),
    fetch('/api/products').then(r => r.json()).catch(() => []),
    fetch('/api/cut_plan').then(r => r.json()),
  ]);
  cut.mats = mats; cut.machine = machine;
  document.getElementById('cut-products').innerHTML = products.map(p => `<option value="${esc(p.code)}">${esc(p.description)}</option>`).join('');
  drawMachine();
  drawLines();
  if (status && status.materials && status.materials.length) { cut.status = status; drawResult(); if (status.state === 'running') pollPlan(); }
}

// ---------- conference ----------
async function cutLoad() {
  const code = document.getElementById('cut-product').value.trim();
  const qty = Math.max(1, parseInt(document.getElementById('cut-qty').value) || 1);
  if (!code) return;
  const r = await fetch(`/api/cut_lines?code=${encodeURIComponent(code)}&qty=${qty}`);
  if (!r.ok) { toast(r.status === 404 ? t('not_found') : t('offline_bar')); return; }
  const d = await r.json();
  const opt = [...document.querySelectorAll('#cut-products option')].find(o => o.value.toUpperCase() === code.toUpperCase());
  cut.product = code; cut.productDesc = opt ? opt.textContent : '';
  cut.lines = d.lines.map(l => ({ ...l, use: true, rot: null, c1: 0, c2: 0, l1: 0, l2: 0 }));
  document.getElementById('cut-warn').innerHTML = (d.warnings || []).map(w => `<div class="warnbox">${esc(t('w_' + w))}</div>`).join('');
  drawLines();
}

function cutAddLine() {
  cut.lines.push({ item: '', code: '', desc: '', material: '', length: 0, width: 0, qty: 1, use: true, rot: null, c1: 0, c2: 0, l1: 0, l2: 0, warnings: [] });
  drawLines();
}

// lineWarnings recomputes the warnings after an edit, from the material list.
function lineWarnings(l) {
  const w = [], m = findMat(l.material);
  if (!m) w.push('unknown_material');
  if (!(num(l.length) > 0)) w.push('no_length');
  if ((!m || m.kind === 'sheet') && !(num(l.width) > 0)) w.push('no_width');
  if (m && num(l.length) > 0) {
    const L = num(l.length), W = num(l.width);
    const fits = m.kind === 'bar' ? L <= m.length : (L <= m.length && W <= m.width) || (!m.grain && W <= m.length && L <= m.width);
    if (!fits) w.push('too_big');
  }
  if ((l.warnings || []).includes('bad_qty')) w.push('bad_qty');
  return w;
}

function drawLines() {
  const el = document.getElementById('cut-table');
  if (!cut.lines.length) { el.innerHTML = `<div class="empty">${esc(t('cut_none'))}</div>`; drawChoose(); return; }
  const h = ['cut_c_use', 'cut_c_item', 'cut_c_code', 'cut_c_desc', 'cut_c_mat', 'cut_c_kind', 'cut_c_len', 'cut_c_wid', '', 'cut_c_qty', 'cut_c_rot'];
  let html = `<table class="conf"><thead><tr>${h.map(k => `<th>${k ? esc(t(k)) : ''}</th>`).join('')}
    <th>C1</th><th>C2</th><th>L1</th><th>L2</th><th>${esc(t('cut_c_warn'))}</th></tr></thead><tbody>`;
  cut.lines.forEach((l, i) => {
    const m = findMat(l.material);
    const rot = l.rot === null ? !(m && m.grain) : l.rot;
    const warn = lineWarnings(l).map(w => w === 'unknown_material'
      ? `${esc(t('w_' + w))} (<a onclick="matOpen('${esc(l.material)}')">${esc(t('cut_register'))}</a>)` : esc(t('w_' + w))).join(' · ');
    const inp = (f, cls) => `<input class="${cls || ''}" value="${esc(l[f] || (cls ? '' : ''))}" onchange="cutEdit(${i}, '${f}', this.value)">`;
    html += `<tr class="${l.use ? '' : 'off'}">
      <td><input type="checkbox" ${l.use ? 'checked' : ''} onchange="cutEdit(${i}, 'use', this.checked)" style="width:auto"></td>
      <td>${inp('item', 'b')}</td><td>${inp('code')}</td><td>${inp('desc')}</td><td>${inp('material')}</td>
      <td>${m ? esc(t('kind_' + m.kind)) + (m.thickness ? ' ' + fmtN(m.thickness) : '') : ''}</td>
      <td>${inp('length', 'n')}</td><td>${inp('width', 'n')}</td>
      <td><button title="${esc(t('cut_swap'))}" onclick="cutSwap(${i})" style="padding: 2px 6px">⇄</button></td>
      <td>${inp('qty', 'b')}</td>
      <td><input type="checkbox" ${rot ? 'checked' : ''} ${m && m.kind === 'bar' ? 'disabled' : ''} onchange="cutEdit(${i}, 'rot', this.checked)" style="width:auto"></td>
      <td>${inp('c1', 'b')}</td><td>${inp('c2', 'b')}</td><td>${inp('l1', 'b')}</td><td>${inp('l2', 'b')}</td>
      <td class="w">${warn}</td></tr>`;
  });
  el.innerHTML = html + '</tbody></table>';
  drawChoose();
}

function cutEdit(i, f, v) {
  const l = cut.lines[i];
  if (['length', 'width', 'c1', 'c2', 'l1', 'l2'].includes(f)) v = num(v);
  if (f === 'qty') { v = Math.max(0, parseInt(v) || 0); l.warnings = (l.warnings || []).filter(w => w !== 'bad_qty'); }
  l[f] = v;
  drawLines();
}
function cutSwap(i) {
  const l = cut.lines[i];
  [l.length, l.width] = [l.width, l.length];
  [l.c1, l.c2, l.l1, l.l2] = [l.l1, l.l2, l.c1, l.c2];
  drawLines();
}

function drawChoose() {
  const el = document.getElementById('cut-choose');
  const prev = {};
  el.querySelectorAll('input').forEach(c => prev[c.value] = c.checked);
  const seen = new Map();
  cut.lines.filter(l => l.use).forEach(l => { const m = findMat(l.material); if (m && !seen.has(m.code)) seen.set(m.code, m); });
  el.innerHTML = [...seen.values()].map(m => `<label class="chk"><input type="checkbox" value="${esc(m.code)}" ${prev[m.code] === false ? '' : 'checked'}>
    ${esc(m.code)} · ${esc(m.description || '')} ${m.thickness ? fmtN(m.thickness) + ' mm' : ''}</label>`).join('') || '<span class="muted">—</span>';
}

// ---------- machine ----------
function drawMachine() {
  const m = cut.machine;
  const f = (key, id, val) => `<div><label>${esc(t(key))}</label><input id="${id}" value="${esc(val ?? '')}"></div>`;
  document.getElementById('cut-machine').innerHTML = `
    ${f('cut_saw_kerf', 'mc-saw-kerf', m.saw_kerf)}${f('cut_edge_trim', 'mc-edge', m.edge_trim)}
    <div><label>${esc(t('cut_stages'))}</label><select id="mc-stages"><option>2</option><option>3</option></select></div>
    <div><label>${esc(t('cut_first'))}</label><select id="mc-first">
      <option value="">${esc(t('cut_first_auto'))}</option><option value="length">${esc(t('cut_first_length'))}</option>
      <option value="width">${esc(t('cut_first_width'))}</option></select></div>
    ${f('cut_bar_kerf', 'mc-bar-kerf', m.bar_kerf)}${f('cut_start_loss', 'mc-start', m.start_loss)}${f('cut_end_loss', 'mc-end', m.end_loss)}
    ${f('cut_time', 'mc-time', m.time_limit_sec || '')}${f('cut_order', 'mc-order', m.order)}${f('cut_user', 'mc-user', m.user)}`;
  document.getElementById('mc-stages').value = String(m.stages === 2 ? 2 : 3);
  document.getElementById('mc-first').value = m.first_cut || '';
}
function readMachine() {
  const g = id => document.getElementById(id).value;
  return { saw_kerf: num(g('mc-saw-kerf')), edge_trim: num(g('mc-edge')), stages: parseInt(g('mc-stages')), first_cut: g('mc-first'),
    bar_kerf: num(g('mc-bar-kerf')), start_loss: num(g('mc-start')), end_loss: num(g('mc-end')),
    time_limit_sec: parseInt(g('mc-time')) || 0, order: g('mc-order').trim(), user: g('mc-user').trim() };
}

// ---------- run ----------
async function cutRun() {
  const lines = cut.lines.filter(l => l.use).map(l => {
    const o = { item: l.item, code: l.code, desc: l.desc, material: l.material, length: num(l.length), width: num(l.width),
      qty: parseInt(l.qty) || 0, c1: num(l.c1), c2: num(l.c2), l1: num(l.l1), l2: num(l.l2) };
    if (l.rot !== null) o.can_rotate = !!l.rot;
    return o;
  });
  const materials = [...document.querySelectorAll('#cut-choose input:checked')].map(c => c.value);
  if (!lines.length || !materials.length) return;
  cut.machine = readMachine();
  const req = { product: cut.product, product_desc: cut.productDesc, product_qty: Math.max(1, parseInt(document.getElementById('cut-qty').value) || 1),
    lines, machine: cut.machine, materials };
  const r = await fetch('/api/cut_plan', { method: 'POST', body: JSON.stringify(req) });
  if (!r.ok) { toast(await r.text()); return; }
  cut.status = await r.json();
  drawResult();
  pollPlan();
}
async function cutStop() { await fetch('/api/cut_plan/stop', { method: 'POST' }); }
function pollPlan() {
  clearTimeout(cut.poll);
  cut.poll = setTimeout(async () => {
    cut.status = await (await fetch('/api/cut_plan')).json();
    drawResult();
    if (cut.status.state === 'running') pollPlan();
  }, 600);
}
// The report opens in a new window; when a reports folder is set it is
// saved there too, and a failure to save is shown.
async function cutPDF() {
  const r = await fetch('/api/cut_plan/pdf');
  if (!r.ok) { toast(await r.text()); return; }
  const err = r.headers.get('X-Save-Error');
  if (err) toast(t('cut_save_error', err));
  window.open(URL.createObjectURL(await r.blob()), '_blank');
}
async function cutXLSX() {
  const r = await fetch('/api/cut_plan/xlsx');
  if (!r.ok) { toast(await r.text()); return; }
  const err = r.headers.get('X-Save-Error');
  if (err) toast(t('cut_save_error', err));
  const a = document.createElement('a');
  a.href = URL.createObjectURL(await r.blob()); a.download = fileNameOf(r, 'resumo.xlsx'); a.click();
}

// ---------- results ----------
const PALETTE = ['#cfe3ff', '#ffe0b8', '#d4f5d4', '#f6d3f0', '#fff3b0', '#d8d4ff', '#c8f0ee', '#ffd6d6'];
function colourOf(id, parts) { const i = parts.findIndex(p => p.id === id); return PALETTE[(i < 0 ? 0 : i) % PALETTE.length]; }

// groupSheets puts identical layouts together, in order of first use.
function groupSheets(sheets) {
  const groups = [];
  const byKey = {};
  sheets.forEach(s => {
    const key = s.stock_id + '|' + s.length + '|' + s.width + '|' + s.placements.map(p => `${p.part_id}@${p.x},${p.y}${p.rotated ? 'r' : ''}`).join(';');
    if (byKey[key]) byKey[key].count++;
    else { byKey[key] = { sheet: s, count: 1 }; groups.push(byKey[key]); }
  });
  return groups;
}

// useOf is the parts over the whole sheets or bars, as the report prints it.
function useOf(mp) {
  const res = mp.result;
  if (mp.planner === 'linear') {
    const total = res.bars.reduce((a, b) => a + b.length, 0);
    return total ? res.bars.reduce((a, b) => a + b.cuts.reduce((x, c) => x + c.length, 0), 0) / total : 0;
  }
  return sheetUse(res.sheets, mp.parts);
}
function sheetUse(sheets, parts) {
  const size = {}; parts.forEach(p => size[p.id] = p.cut_length * p.cut_width);
  const total = sheets.reduce((a, s) => a + s.length * s.width, 0);
  return total ? sheets.reduce((a, s) => a + s.placements.reduce((x, p) => x + (size[p.part_id] || 0), 0), 0) / total : 0;
}

function drawResult() {
  const st = cut.status;
  const run = st && st.state === 'running';
  document.getElementById('cut-run').disabled = run;
  document.getElementById('cut-stop').disabled = !run;
  document.getElementById('cut-pdf').disabled = document.getElementById('cut-xlsx').disabled = run || !st || !st.materials || !st.materials.some(m => m.state === 'done');
  document.getElementById('cut-run-state').textContent = run ? t('cut_state_running') : '';
  if (!st) return;
  let html = '';
  if (st.problems && st.problems.length) html += `<div class="warnbox"><b>${esc(t('cut_left_out'))}:</b> ${st.problems.map(pr => esc(pr.code + ': ' + t('lp_' + pr.reason, pr.material))).join('; ')}</div>`;
  (st.materials || []).forEach(mp => {
    const m = mp.material, res = mp.result;
    const verdict = mp.state !== 'done' ? '' : (mp.problems.length ? `<span class="bad">${esc(t('cut_refused'))}: ${mp.problems.map(esc).join('; ')}</span>`
      : `<span class="ok">✔ ${esc(t('cut_verified'))}</span>`);
    let body = '';
    if (res) {
      const unplaced = (res.unplaced || []).map(u => `${u.part_id} ×${u.quantity}`).join(', ');
      if (unplaced) body += `<div class="warnbox">${esc(t('cut_unplaced', unplaced))}</div>`;
      body += mp.planner === 'linear' ? drawBarsText(res) : drawSheets(res, mp.parts);
    }
    const count = res ? (mp.planner === 'linear' ? t('cut_bars_n', res.bars.length) : t('cut_sheets', res.sheets.length)) : '';
    html += `<div class="card"><h3>${esc(m.code)} · ${esc(m.description || '')} ${m.thickness ? fmtN(m.thickness) + ' mm' : ''}</h3>
      <div class="row muted">${esc(t('cut_state_' + mp.state))} ${res ? '· ' + esc(count) + ' · ' + esc(t('cut_util', fmtN(useOf(mp) * 100))) : ''}
      ${mp.error ? '· <span class="bad">' + esc(mp.error) + '</span>' : ''} <span class="grow"></span>${verdict}</div>${body}</div>`;
  });
  document.getElementById('cut-result').innerHTML = html;
}

function drawBarsText(res) {
  const groups = [], byKey = {};
  res.bars.forEach(b => {
    const key = b.stock_id + '|' + b.cuts.map(c => c.length).join('+');
    if (byKey[key]) byKey[key].count++; else { byKey[key] = { bar: b, count: 1 }; groups.push(byKey[key]); }
  });
  let n = 0;
  return `<div class="bars">${groups.map(g => {
    const from = n + 1; n += g.count;
    const b = g.bar;
    return `${esc(t('cut_bar'))} ${from}${g.count > 1 ? '–' + n : ''} (${esc(b.stock_id)} ${fmtN(b.length)}): ${b.cuts.map(c => fmtN(c.length)).join(' + ')}, ` +
      `${esc(t(b.reusable ? 'cut_offcut' : 'cut_scrap'))} ${fmtN(b.remainder)}`;
  }).join('<br>')}</div>`;
}

function drawSheets(res, parts) {
  const groups = groupSheets(res.sheets);
  return groups.map((g, gi) => {
    const s = g.sheet, L = s.length, W = s.width, sw = Math.max(L, W) / 600;
    const partOf = id => parts.find(p => p.id === id) || { cut_length: 0, cut_width: 0 };
    let svg = `<svg viewBox="${-sw * 4} ${-sw * 4} ${L + sw * 8} ${W + sw * 8}">
      <rect x="0" y="0" width="${L}" height="${W}" fill="#e9ecf1" stroke="#555" stroke-width="${sw}"/>`;
    (s.offcuts || []).forEach(o => {
      svg += `<rect x="${o.x}" y="${o.y}" width="${o.length}" height="${o.width}" fill="${o.reusable ? '#c9efd2' : '#eee'}" stroke="#999" stroke-width="${sw / 2}"/>`;
      if (o.reusable && o.length > L / 12 && o.width > W / 14) svg += `<text x="${o.x + o.length / 2}" y="${o.y + o.width / 2}" style="font-size:${Math.min(L / 50, o.width / 3)}px"
        text-anchor="middle" dominant-baseline="middle" fill="#1a7f37">${fmtN(o.length)} × ${fmtN(o.width)}</text>`;
    });
    s.placements.forEach((p, i) => {
      const jp = partOf(p.part_id);
      const dx = p.rotated ? jp.cut_width : jp.cut_length, dy = p.rotated ? jp.cut_length : jp.cut_width;
      const fs = Math.min(dx / 7, dy / 3.2, L / 55);
      svg += `<rect x="${p.x}" y="${p.y}" width="${dx}" height="${dy}" fill="${colourOf(p.part_id, parts)}" stroke="#333" stroke-width="${sw}"/>
        <text x="${p.x + dx / 2}" y="${p.y + dy / 2}" style="font-size:${fs}px" text-anchor="middle" fill="#111">
          <tspan x="${p.x + dx / 2}" dy="${-fs * 0.2}" font-weight="700" style="font-size:${fs}px">${i + 1}</tspan>
          <tspan x="${p.x + dx / 2}" dy="${fs * 1.1}" style="font-size:${fs}px">${esc(p.part_id)}</tspan>
          <tspan x="${p.x + dx / 2}" dy="${fs * 1.1}" style="font-size:${fs * 0.85}px">${fmtN(dx)} × ${fmtN(dy)}</tspan></text>`;
    });
    svg += '</svg>';
    return `<div class="res-item"><div class="row"><b>${esc(t('cut_plan_n', gi + 1, groups.length))}</b>
      <span class="muted">${esc(s.stock_id)} · ${fmtN(L)} × ${fmtN(W)} · ${esc(t('cut_times', g.count))} · ${esc(t('cut_util', fmtN(sheetUse([s], parts) * 100)))}</span></div>${svg}</div>`;
  }).join('');
}

// ---------- material list ----------
function matOpen(newCode) {
  document.getElementById('m-min-l').value = cut.mats.min_offcut_length || '';
  document.getElementById('m-min-w').value = cut.mats.min_offcut_width || '';
  document.getElementById('m-body').innerHTML = '';
  document.getElementById('m-error').innerHTML = '';
  cut.mats.materials.forEach(matAddRow);
  if (newCode) matAddRow({ code: newCode, kind: 'sheet' }, true);
  showModal('mat-modal');
}
// matImport reads a spreadsheet on the server and shows the merged list
// here, unsaved: the person checks it and clicks Save.
async function matImport(input) {
  const f = input.files[0];
  input.value = '';
  if (!f) return;
  const r = await fetch('/api/materials/import?name=' + encodeURIComponent(f.name), { method: 'POST', body: f });
  const box = document.getElementById('m-error');
  if (!r.ok) { box.innerHTML = `<div class="warnbox">${esc(await r.text())}</div>`; return; }
  const d = await r.json();
  if (d.missing.length) { box.innerHTML = `<div class="warnbox">${esc(t('m_import_missing', d.missing.join(', ')))}</div>`; return; }
  document.getElementById('m-body').innerHTML = '';
  d.list.materials.forEach(m => matAddRow(m));
  let html = `<div class="warnbox" style="background:#0f2e22;border-color:#059669;color:#a7f3d0">${esc(t('m_imported', d.added, d.updated))}</div>`;
  const why = pr => (pr.row ? t('m_row', pr.row) + (pr.code ? ` (${pr.code})` : '') + ': ' : pr.code + ': ') + t('mi_' + pr.reason, pr.value || '');
  if (d.problems.length) html += `<div class="warnbox"><b>${esc(t('m_import_rows'))}</b><br>${d.problems.map(pr => esc(why(pr))).join('<br>')}</div>`;
  box.innerHTML = html;
}
async function matExport(format, template) {
  const r = await fetch(`/api/materials/export?format=${format}${template ? '&template=1' : ''}`);
  if (!r.ok) { toast(await r.text()); return; }
  const a = document.createElement('a');
  a.href = URL.createObjectURL(await r.blob()); a.download = fileNameOf(r, 'materiais.' + format); a.click();
}

function matAddRow(m, focus) {
  m = m || { kind: 'sheet' };
  const offcuts = (m.offcuts || []).map(o => (m.kind === 'bar' ? `${o.length}` : `${o.length}x${o.width}`) + (o.quantity > 1 ? `x${o.quantity}` : '')).join('; ');
  const tr = document.createElement('tr');
  const v = x => esc(x ?? '');
  tr.innerHTML = `<td><input class="m-code" value="${v(m.code)}" style="width: 100px"></td><td><input class="m-alt" value="${v(m.alt_code)}" style="width: 90px"></td>
    <td><input class="m-desc" value="${v(m.description)}" style="width: 220px"></td>
    <td><select class="m-kind"><option value="sheet">${esc(t('kind_sheet'))}</option><option value="bar">${esc(t('kind_bar'))}</option></select></td>
    <td><input class="m-thick" value="${v(m.thickness || '')}" style="width: 50px"></td>
    <td><input type="checkbox" class="m-grain" ${m.grain ? 'checked' : ''} style="width: auto"></td>
    <td><input class="m-len" value="${v(m.length || '')}" style="width: 65px"></td><td><input class="m-wid" value="${v(m.width || '')}" style="width: 65px"></td>
    <td><input class="m-minl" value="${v(m.min_offcut_length || '')}" style="width: 55px"> <input class="m-minw" value="${v(m.min_offcut_width || '')}" style="width: 55px"></td>
    <td><input class="m-off" value="${v(offcuts)}" placeholder="${esc(t('m_offcuts_ph'))}" style="width: 170px"></td>
    <td><button onclick="this.closest('tr').remove()">✕</button></td>`;
  tr.querySelector('.m-kind').value = m.kind || 'sheet';
  document.getElementById('m-body').appendChild(tr);
  if (focus) tr.querySelector('.m-desc').focus();
}
async function matSave() {
  const rows = [...document.querySelectorAll('#m-body tr')];
  const list = {
    min_offcut_length: num(document.getElementById('m-min-l').value), min_offcut_width: num(document.getElementById('m-min-w').value),
    materials: rows.map(tr => {
      const q = c => tr.querySelector(c);
      const kind = q('.m-kind').value;
      const offcuts = q('.m-off').value.split(';').map(s => s.trim()).filter(Boolean).map(s => {
        const n = s.toLowerCase().split(/[x×*]/).map(num);
        return kind === 'bar' ? { length: n[0], quantity: n[1] || 1 } : { length: n[0], width: n[1] || 0, quantity: n[2] || 1 };
      });
      return { code: q('.m-code').value.trim(), alt_code: q('.m-alt').value.trim(), description: q('.m-desc').value.trim(), kind,
        thickness: num(q('.m-thick').value), grain: q('.m-grain').checked, length: num(q('.m-len').value), width: num(q('.m-wid').value),
        min_offcut_length: num(q('.m-minl').value), min_offcut_width: num(q('.m-minw').value), offcuts };
    }).filter(m => m.code || m.description)
  };
  const r = await fetch('/api/materials', { method: 'POST', body: JSON.stringify(list) });
  if (!r.ok) {
    document.getElementById('m-error').innerHTML = `<div class="warnbox">${(await r.text()).split('\n').map(esc).join('<br>')}</div>`;
    return;
  }
  cut.mats = await r.json();
  closeModal('mat-modal');
  toast(t('m_saved'));
  drawLines();
}
