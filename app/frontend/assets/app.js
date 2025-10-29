const API = "/api";

async function httpJSON(url, options) {
    try {
        const res = await fetch(url, options);
        if (!res.ok) {
            const body = await res.text().catch(() => "");
            throw { kind: "http", status: res.status, statusText: res.statusText, body };
        }
        const ct = res.headers.get("content-type") || "";
        if (!ct.includes("application/json")) {
            const body = await res.text().catch(() => "");
            throw { kind: "invalid-json", body };
        }
        return await res.json();
    } catch (e) {
        if (e.kind) throw e;
        throw { kind: "network", message: String(e && e.message ? e.message : e) };
    }
}

function snippet(s, n = 200) {
    return (s || "").slice(0, n);
}

function renderErrorCard(title, detailsHtml) {
    return `
    <div class="card bevel">
      <h2 class="rainbow-text big">${escapeHTML(title)}</h2>
      <p class="error">Could not reach the API.</p>
      ${detailsHtml ? `<p class="mono">${detailsHtml}</p>` : ""}
      <p><a class="loud-link" href="#/new">Create a post</a> or try again.</p>
    </div>
  `;
}

function errorDetailsHTML(err) {
    if (!err || typeof err !== "object") return escapeHTML(String(err));
    if (err.kind === "http") {
        const extra = err.body ? ` — ${escapeHTML(snippet(err.body))}` : "";
        return `HTTP ${err.status} ${escapeHTML(err.statusText || "")}${extra}`;
    }
    if (err.kind === "invalid-json") {
        const extra = err.body ? `: ${escapeHTML(snippet(err.body))}` : "";
        return `Invalid JSON${extra}`;
    }
    if (err.kind === "network") return `Network error`;
    return escapeHTML(JSON.stringify(err));
}

class BlogApp extends HTMLElement {
    constructor() {
        super();
        this.innerHTML = `
      <div class="app-card bevel">
        <div class="titlebar">
          <span class="title">📟 Posts</span>
        </div>
        <div id="view" class="content-area"></div>
      </div>
    `;
        this.view = this.querySelector("#view");
        window.addEventListener("hashchange", () => this.route());
    }
    connectedCallback() { this.route(); }

    route() {
        const hash = location.hash || "#/";
        if (hash.startsWith("#/new")) return this.renderNew();
        if (hash.startsWith("#/post/")) {
            const id = decodeURIComponent(hash.replace("#/post/", ""));
            return this.renderPost(id);
        }
        return this.renderList();
    }

    async renderList() {
        this.view.innerHTML = `<div class="loading blink">Loading posts…</div>`;
        try {
            const posts = await httpJSON(`${API}/posts`);
            if (!Array.isArray(posts) || posts.length === 0) {
                this.view.innerHTML = `
          <h2 class="rainbow-text big">Latest Posts</h2>
          <p>No posts yet… <a class="loud-link" href="#/new">click here to create one</a>.</p>
        `;
                return;
            }
            this.view.innerHTML = `
        <h2 class="rainbow-text big">Latest Posts</h2>
        <table class="table-90s">
          <thead><tr><th>Title</th><th>Published</th></tr></thead>
          <tbody>
            ${posts.map(p => `
              <tr>
                <td><a class="loud-link" href="#/post/${encodeURIComponent(p.id)}">${escapeHTML(p.title)}</a></td>
                <td>${new Date(p.created_at * 1000).toLocaleString()}</td>
              </tr>`).join("")}
          </tbody>
        </table>
      `;
        } catch (err) {
            this.view.innerHTML = renderErrorCard("Latest Posts", errorDetailsHTML(err));
        }
    }

    async renderPost(id) {
        this.view.innerHTML = `<div class="loading blink">Loading post…</div>`;
        try {
            const p = await httpJSON(`${API}/posts/${encodeURIComponent(id)}`);
            this.view.innerHTML = `
        <div class="post-window bevel">
          <div class="titlebar">
            <span>📝 ${escapeHTML(p.title)}</span>
            <span class="mini-led blink">REC</span>
          </div>
          <div class="post-body">
            <img class="post-image funky-border"
                 src="${API}/images/${encodeURIComponent(p.id)}"
                 alt="${escapeHTML(p.title)} image"
                 onerror="this.remove()"/>
            <pre class="body mono">${escapeHTML(p.body)}</pre>
          </div>
          <p><a class="loud-link" href="#/">← Back</a></p>
        </div>
      `;
        } catch (err) {
            this.view.innerHTML = renderErrorCard("Post", errorDetailsHTML(err));
        }
    }

    renderNew() {
        this.view.innerHTML = `
      <form id="postForm" class="card form-90s bevel">
        <h2 class="rainbow-text">Create New Post</h2>
        <label>Title <input name="title" required /></label>
        <label>Body <textarea name="body" rows="8" required></textarea></label>
        <label>Image <input type="file" name="image" accept="image/*" /></label>
        <div id="effectsSection" style="display:none; margin-top:8px;">
          <strong>Effects</strong>
          <div>
            <label><input type="radio" name="effect" value="none" checked /> None</label>
            <label><input type="radio" name="effect" value="grayscale" /> Grayscale</label>
            <label><input type="radio" name="effect" value="invert" /> Invert</label>
          </div>
          <p class="hint">Effect runs as a Kubernetes Job after upload.</p>
        </div>
        <button type="submit" class="btn-3d btn-yellow">Create</button>
      </form>
      <div id="jobStatus" class="mono"></div>
    `;

        const form = this.view.querySelector("#postForm");
        const effects = this.view.querySelector("#effectsSection");
        const fileInput = form.querySelector('input[name="image"]');
        const statusEl = this.view.querySelector("#jobStatus");

        const toggleEffects = () => {
            const file = fileInput.files && fileInput.files[0];
            effects.style.display = file ? "block" : "none";
            if (!file) {
                form.querySelector('input[name="effect"][value="none"]').checked = true;
            }
        };
        fileInput.addEventListener("change", toggleEffects);
        toggleEffects();

        form.addEventListener("submit", async (e) => {
            e.preventDefault();
            statusEl.textContent = "";
            const fd = new FormData(form);
            const title = fd.get("title");
            const body = fd.get("body");
            const image = fileInput.files && fileInput.files[0] ? fileInput.files[0] : null;
            const effect = (fd.get("effect") || "none").toString();

            let post;
            try {
                post = await httpJSON(`${API}/posts`, {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ title, body })
                });
            } catch (err) {
                statusEl.innerHTML = `<div class="error">Create post failed: ${errorDetailsHTML(err)}</div>`;
                return;
            }

            if (image) {
                try {
                    const imgFd = new FormData();
                    imgFd.set("file", image);
                    const upRes = await fetch(`${API}/images/${post.id}`, { method: "POST", body: imgFd });
                    if (!upRes.ok) {
                        const t = await upRes.text().catch(() => "");
                        statusEl.innerHTML = `<div class="error">Image upload failed: HTTP ${upRes.status} ${escapeHTML(upRes.statusText)} ${escapeHTML(snippet(t))}</div>`;
                        location.hash = `#/post/${encodeURIComponent(post.id)}`;
                        return;
                    }
                } catch (err) {
                    statusEl.innerHTML = `<div class="error">Image upload failed: ${errorDetailsHTML(err)}</div>`;
                    location.hash = `#/post/${encodeURIComponent(post.id)}`;
                    return;
                }

                if (effect !== "none") {
                    statusEl.innerHTML = `<div class="blink">⏳ Starting image job (${escapeHTML(effect)})…</div>`;
                    let jobName;
                    try {
                        const job = await httpJSON(`${API}/jobs/effect`, {
                            method: "POST",
                            headers: { "Content-Type": "application/json" },
                            body: JSON.stringify({ post_id: post.id, effect })
                        });
                        jobName = job.job_name;
                    } catch (err) {
                        statusEl.innerHTML = `<div class="error">Failed to start job: ${errorDetailsHTML(err)}</div>`;
                        return;
                    }
                    await this.pollJobUntilDone(jobName, statusEl, () => {
                        location.hash = `#/post/${encodeURIComponent(post.id)}`;
                    });
                    return;
                }
            }

            location.hash = `#/post/${encodeURIComponent(post.id)}`;
        });
    }

    async pollJobUntilDone(jobName, statusEl, onSuccess) {
        statusEl.innerHTML = `<div class="blink">🏃 Job <b>${escapeHTML(jobName)}</b> running…</div>`;
        let done = false;
        while (!done) {
            try {
                await new Promise(r => setTimeout(r, 1000));
                const st = await httpJSON(`${API}/jobs/${encodeURIComponent(jobName)}/status`);
                if (st.status === "succeeded") {
                    statusEl.innerHTML = `✅ Job <b>${escapeHTML(jobName)}</b> succeeded.`;
                    done = true;
                    onSuccess();
                    break;
                } else if (st.status === "failed") {
                    statusEl.innerHTML = `<div class="error">❌ Job failed: ${escapeHTML(st.reason || "unknown error")}</div>`;
                    done = true;
                    break;
                } else {
                    statusEl.innerHTML = `<div class="blink">🏃 Job <b>${escapeHTML(jobName)}</b> ${escapeHTML(st.status)}…</div>`;
                }
            } catch (err) {
                statusEl.innerHTML = `<div class="error">Failed to poll job: ${errorDetailsHTML(err)}</div>`;
                break;
            }
        }
    }
}

customElements.define("blog-app", BlogApp);

function escapeHTML(s) {
    return String(s)
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;")
        .replaceAll("'", "&#039;");
}
