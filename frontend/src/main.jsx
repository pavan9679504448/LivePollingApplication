import React, { useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { BarChart3, Plus, ArrowRight, ArrowLeft, Check, Copy, X, LogOut, Radio, Users, LockKeyhole, Link2, LoaderCircle, CircleAlert, WifiOff, CheckCircle2, LayoutGrid, ExternalLink } from 'lucide-react';
import './styles.css';

async function api(path, body, method = 'POST') {
  let response;
  try {
    response = await fetch('/api' + path, body === undefined ? { credentials: 'same-origin' } : {
      method, credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-PulsePoll-Request': '1' }, body: JSON.stringify(body),
    });
  } catch { throw new Error('Cannot reach the server. Check your connection and try again.'); }
  const data = await response.json().catch(() => ({}));
  if (!response.ok) { const error = new Error(data.error || 'Something went wrong. Please retry.'); error.status = response.status; throw error; }
  return data;
}

function navigate(path) { history.pushState({}, '', path); window.dispatchEvent(new PopStateEvent('popstate')); window.scrollTo(0, 0); }
function Link({ to, children, ...props }) {
  return <a href={to} {...props} onClick={e => { if (e.button === 0 && !e.metaKey && !e.ctrlKey && !e.shiftKey && !e.altKey) { e.preventDefault(); navigate(to); } }}>{children}</a>;
}
function Brand() { return <Link to="/" className="brand" aria-label="PulsePoll home"><span className="brand-icon"><BarChart3 size={23} strokeWidth={2.8} /></span>pulse<span className="brand-light">poll</span></Link>; }
function Spinner() { return <LoaderCircle size={18} className="spin" aria-hidden="true" />; }
function ErrorBox({ message }) { return message ? <div className="error-box" role="alert"><CircleAlert size={18} /><span>{message}</span></div> : null; }
function State({ title, text, retry }) { return <div className="state"><BarChart3 size={36} /><h2>{title}</h2><p>{text}</p>{retry && <button className="button primary" onClick={retry}>Try again</button>}</div>; }

function App() {
  const [path, setPath] = useState(location.pathname);
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [authRevision, setAuthRevision] = useState(0);
  useEffect(() => { const handler = () => setPath(location.pathname); window.addEventListener('popstate', handler); return () => window.removeEventListener('popstate', handler); }, []);
  useEffect(() => {
    let active = true; setLoading(true); setError('');
    api('/auth/me').then(data => { if (active) setUser(data); }).catch(e => { if (active && e.status !== 401) setError(e.message); }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [authRevision]);
  const pollID = path.match(/^\/poll\/([a-f0-9]{24})$/)?.[1];
  const onAuth = account => { setUser(account); if (path !== '/new') navigate('/'); };
  async function logout() { try { await api('/auth/logout', {}); setUser(null); navigate('/'); } catch (e) { setError(e.message); } }
  if (loading) return <div className="app-loading"><Brand /><Spinner /><span>Opening your workspace…</span></div>;
  if (error) return <div className="auth-page"><Brand /><State title="Let’s reconnect" text={error} retry={() => setAuthRevision(x => x + 1)} /></div>;
  if (pollID) return <PollPage key={pollID} id={pollID} user={user} />;
  if (path !== '/' && path !== '/new') return <div className="auth-page"><Brand /><State title="Page not found" text="Check the poll link or return to your workspace." /><Link to="/" className="button primary">Go to workspace</Link></div>;
  if (!user) return <Auth onAuth={onAuth} />;
  return <div className="workspace">
    <aside className="sidebar"><Brand /><div className="nav-label">WORKSPACE</div><nav><Link className={'nav-link ' + (path === '/' ? 'active' : '')} to="/"><LayoutGrid size={19} />Your polls</Link><Link className={'nav-link ' + (path === '/new' ? 'active' : '')} to="/new"><Plus size={19} />Create a poll</Link></nav>
      <div className="sidebar-bottom"><div className="profile"><span className="avatar">{user.name.slice(0, 1).toUpperCase()}</span><div><strong>{user.name}</strong><span>Your workspace</span></div></div><button className="signout" onClick={logout}><LogOut size={16} />Sign out</button></div>
    </aside>
    <main className="main-content">{path === '/new' ? <CreatePoll /> : <Dashboard user={user} />}</main>
  </div>;
}

function Auth({ onAuth }) {
  const [signup, setSignup] = useState(true), [busy, setBusy] = useState(false), [error, setError] = useState('');
  async function submit(event) {
    event.preventDefault(); setError(''); setBusy(true);
    const values = Object.fromEntries(new FormData(event.currentTarget));
    try { const user = await api('/auth/' + (signup ? 'signup' : 'login'), values); onAuth(user); }
    catch (e) { setError(e.message); } finally { setBusy(false); }
  }
  return <div className="auth-layout">
    <section className="auth-intro"><Brand /><div className="intro-content"><span className="eyebrow">A LITTLE QUESTION. A LOT OF VOICES.</span><h1>Ask together.<br /><span>Decide together.</span></h1><p>Create a question, share one link, and watch the answers arrive.</p><div className="intro-steps"><span><span>01</span> Create your poll</span><span><span>02</span> Invite everyone</span><span><span>03</span> See results live</span></div></div><div className="intro-foot"><Radio size={17} />Every vote brings a clearer picture.</div></section>
    <section className="auth-form-wrap"><div className="auth-form"><span className="eyebrow blue">YOUR NEXT QUESTION STARTS HERE</span><h2>{signup ? 'Make room for every voice.' : 'Welcome back.'}</h2><p>{signup ? 'Create your account to start a poll.' : 'Sign in to your polling workspace.'}</p><div className="auth-tabs" aria-label="Account action"><button className={signup ? 'selected' : ''} onClick={() => { setSignup(true); setError(''); }}>Create account</button><button className={!signup ? 'selected' : ''} onClick={() => { setSignup(false); setError(''); }}>Sign in</button></div>
      <form key={String(signup)} onSubmit={submit}>
        {signup && <label>Your name<input name="name" placeholder="e.g. Pavan" autoComplete="name" required minLength={2} maxLength={60} /></label>}
        <label>Email address<input name="email" type="email" placeholder="you@example.com" autoComplete="email" required maxLength={254} /></label>
        <label>Password<input name="password" type="password" placeholder={signup ? 'At least 10 characters' : 'Enter your password'} autoComplete={signup ? 'new-password' : 'current-password'} required minLength={signup ? 10 : 1} maxLength={72} /></label>
        <ErrorBox message={error} /><button className="button primary full" disabled={busy}>{busy ? <Spinner /> : signup ? 'Create account' : 'Sign in'}{!busy && <ArrowRight size={18} />}</button>
      </form><p className="auth-note"><LockKeyhole size={15} />An account is needed to create polls. Anyone with a poll link can vote.</p>
    </div></section>
  </div>;
}

function Dashboard({ user }) {
  const [polls, setPolls] = useState([]), [loading, setLoading] = useState(true), [error, setError] = useState(''), [filter, setFilter] = useState('all'), [revision, setRevision] = useState(0);
  useEffect(() => { let active = true; setLoading(true); setError(''); api('/polls').then(data => { if (active) setPolls(data.polls); }).catch(e => { if (active) setError(e.message); }).finally(() => { if (active) setLoading(false); }); return () => { active = false; }; }, [revision]);
  const filtered = polls.filter(p => filter === 'all' || (filter === 'open' ? p.open : !p.open));
  return <><header className="page-header"><div><span className="eyebrow">YOUR WORKSPACE</span><h1>Your polls<span className="title-dot">.</span></h1><p>{polls.length ? 'Small questions. Shared decisions.' : `Hi ${user.name.split(' ')[0]}, what would you like to ask?`}</p></div><Link to="/new" className="button primary"><Plus size={18} />Create a poll</Link></header>
    <div className="stats-strip"><div><span>Polls created</span><strong>{polls.length}</strong></div><div><span>Open polls</span><strong>{polls.filter(p => p.open).length}</strong></div><div><span>Votes received</span><strong>{polls.reduce((sum, p) => sum + p.total, 0).toLocaleString()}</strong></div></div>
    <div className="list-toolbar"><div className="filter-tabs">{[['all', 'All polls'], ['open', 'Open'], ['closed', 'Closed']].map(([value, label]) => <button key={value} className={filter === value ? 'selected' : ''} onClick={() => setFilter(value)}>{label}</button>)}</div><span className="muted">{polls.length >= 100 ? 'Latest 100 polls' : `${filtered.length} ${filtered.length === 1 ? 'poll' : 'polls'}`}</span></div>
    {loading ? <div className="state"><Spinner /><p>Loading your polls…</p></div> : error ? <State title="Couldn’t load your polls" text={error} retry={() => setRevision(x => x + 1)} /> : !filtered.length ? <div className="empty-state"><div className="empty-icon"><BarChart3 size={37} /></div><h2>{polls.length ? `No ${filter} polls yet` : 'Your first poll starts here'}</h2><p>{polls.length ? 'Your polls will appear here when their status changes.' : 'Ask your class, your team, or your friends. All you need is one good question.'}</p><Link to="/new" className="button primary"><Plus size={18} />Create a poll</Link></div> : <div className="poll-grid">{filtered.map(p => <PollCard key={p.id} poll={p} />)}</div>}
  </>;
}

function PollCard({ poll }) {
  return <Link to={'/poll/' + poll.id} className="poll-card"><div className="card-top"><span className={'badge ' + (poll.open ? 'open' : '')}>{poll.open ? 'Open' : 'Closed'}</span><span className="muted">{new Date(poll.createdAt).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}</span></div><h2>{poll.question}</h2><p className="card-description">{poll.description || `${poll.options.length} options · One choice per browser`}</p><div className="mini-bars" aria-hidden="true">{poll.options.map((o, i) => <span key={o.id} style={{ width: `${poll.total ? o.votes / poll.total * 100 : 100 / poll.options.length}%`, background: colors[i] }} />)}</div><div className="card-bottom"><span><Users size={16} />{poll.total.toLocaleString()} {poll.total === 1 ? 'vote' : 'votes'}</span><span>View poll <ArrowRight size={16} /></span></div></Link>;
}

const colors = ['#315cf5', '#7d99ff', '#22a69f', '#eba657', '#a882d4', '#da7893'];
function CreatePoll() {
  const [question, setQuestion] = useState(''), [description, setDescription] = useState(''), [options, setOptions] = useState(['', '']), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  async function submit(event) { event.preventDefault(); setError(''); setBusy(true); try { const poll = await api('/polls', { question, description, options }); navigate('/poll/' + poll.id); } catch (e) { setError(e.message); } finally { setBusy(false); } }
  return <><Link to="/" className="back-link"><ArrowLeft size={16} />Your polls</Link><header className="page-header"><div><span className="eyebrow">LET’S ASK SOMETHING</span><h1>Create a poll<span className="title-dot">.</span></h1><p>A clear question makes a great conversation.</p></div></header><div className="composer-layout"><form className="panel composer" onSubmit={submit}><div className="section-heading"><span className="step-number">1</span><h2>The question</h2></div><label>What would you like to ask?<textarea rows={2} value={question} onChange={e => setQuestion(e.target.value)} placeholder="e.g. Which topic should we cover next?" required minLength={5} maxLength={200} /></label><div className="field-count">{question.length}/200</div><label>Description <span className="optional">optional</span><textarea rows={2} value={description} onChange={e => setDescription(e.target.value)} placeholder="Add a little context for your audience…" maxLength={400} /></label><div className="section-heading options-heading"><span className="step-number">2</span><h2>The options</h2><span className="muted">2–6 choices</span></div><div className="options-editor">{options.map((option, i) => <div className="option-input" key={i}><span className="option-letter">{String.fromCharCode(65 + i)}</span><input aria-label={`Option ${i + 1}`} value={option} onChange={e => setOptions(options.map((o, j) => i === j ? e.target.value : o))} placeholder={`Option ${i + 1}`} required maxLength={80} />{options.length > 2 && <button type="button" className="icon-button" aria-label={`Remove option ${i + 1}`} onClick={() => setOptions(options.filter((_, j) => j !== i))}><X size={17} /></button>}</div>)}</div><button className="add-option" type="button" disabled={options.length >= 6} onClick={() => setOptions([...options, ''])}><Plus size={17} />Add another option</button><ErrorBox message={error} /><div className="form-footer"><span><Link2 size={16} />You’ll get a link to share.</span><button className="button primary" disabled={busy}>{busy ? <Spinner /> : 'Create poll'}{!busy && <ArrowRight size={18} />}</button></div></form>
    <aside className="preview-column"><div className="preview-heading"><span className="eyebrow">AUDIENCE PREVIEW</span><span className="badge">Preview</span></div><div className="panel preview-card"><span className="eyebrow blue">HAVE YOUR SAY</span><h2>{question.trim() || 'Your question goes here'}</h2>{description && <p>{description}</p>}<div className="preview-options">{options.map((option, i) => <div key={i}><span className="radio-circle" />{option.trim() || `Option ${i + 1}`}</div>)}</div><div className="preview-button">Submit vote</div></div><div className="preview-tip"><Radio size={18} /><p>Your audience can vote from any device. Results update as answers arrive.</p></div></aside></div></>;
}

function PollPage({ id, user }) {
  const [poll, setPoll] = useState(null), [error, setError] = useState(''), [loadError, setLoadError] = useState(''), [choice, setChoice] = useState(''), [busy, setBusy] = useState(false), [closing, setClosing] = useState(false), [connection, setConnection] = useState('connecting'), [revision, setRevision] = useState(0);
  const applyPoll = next => setPoll(previous => !previous || next.version >= previous.version ? next : previous);
  useEffect(() => {
    let source, cancelled = false; setLoadError(''); setConnection('connecting');
    api('/polls/' + id).then(data => {
      if (cancelled) return; applyPoll(data);
      source = new EventSource('/api/polls/' + id + '/events');
      source.onopen = () => setConnection('live'); source.onerror = () => setConnection('reconnecting');
      source.addEventListener('results', event => { try { applyPoll(JSON.parse(event.data)); setConnection('live'); } catch { setConnection('reconnecting'); } });
    }).catch(e => { if (!cancelled) setLoadError(e.message); });
    return () => { cancelled = true; source?.close(); };
  }, [id, revision]);
  async function vote(event) { event.preventDefault(); setBusy(true); setError(''); try { applyPoll(await api('/polls/' + id + '/vote', { optionId: choice })); } catch (e) { setError(e.message); } finally { setBusy(false); } }
  async function close() { if (!window.confirm('Close this poll? New votes will stop, and the results will remain visible.')) return; setClosing(true); setError(''); try { applyPoll(await api('/polls/' + id + '/close', {})); } catch (e) { setError(e.message); } finally { setClosing(false); } }
  return <div className="public-page"><header className="public-header"><Brand /><Link to="/" className="button secondary">{user ? 'Your polls' : 'Create your own poll'}<ArrowRight size={16} /></Link></header><main className="poll-main">
    {loadError ? <State title="Couldn’t open this poll" text={loadError} retry={() => setRevision(x => x + 1)} /> : !poll ? <div className="state"><Spinner /><p>Opening the poll…</p></div> : <><div className="poll-topline"><span className="eyebrow">{poll.open ? 'EVERY VOICE COUNTS' : 'THE RESULTS ARE IN'}</span><span className={'connection ' + connection}>{connection === 'live' ? <Radio size={15} /> : connection === 'reconnecting' ? <WifiOff size={15} /> : <Spinner />}{connection === 'live' ? 'Live connection' : connection === 'reconnecting' ? 'Reconnecting…' : 'Connecting…'}</span></div><div className="voting-layout"><section className="panel voting-card"><span className={'badge ' + (poll.open ? 'open' : '')}>{poll.open ? 'Open for votes' : 'Poll closed'}</span><h1>{poll.question}</h1>{poll.description && <p className="poll-description">{poll.description}</p>}<form onSubmit={vote}><fieldset disabled={busy || !!poll.myChoice || !poll.open || poll.total >= poll.capacity}><legend className="sr-only">Choose one option</legend><div className="vote-options">{poll.options.map((option, i) => <label key={option.id} className={'vote-option ' + ((poll.myChoice || choice) === option.id ? 'chosen' : '')}><input type="radio" name="vote" value={option.id} checked={(poll.myChoice || choice) === option.id} onChange={() => setChoice(option.id)} /><span className="option-letter">{String.fromCharCode(65 + i)}</span><span>{option.label}</span>{poll.myChoice === option.id && <Check size={19} />}</label>)}</div></fieldset><ErrorBox message={error} />{poll.myChoice ? <div className="vote-confirmation" role="status"><CheckCircle2 size={19} />Your vote is in. Thanks for taking part.</div> : !poll.open ? <div className="closed-note"><LockKeyhole size={17} />Voting has ended. You can still view the results.</div> : poll.total >= poll.capacity ? <div className="closed-note">This poll has reached its vote limit.</div> : <><button className="button primary full vote-submit" disabled={!choice || busy}>{busy ? <Spinner /> : 'Submit vote'}{!busy && <ArrowRight size={18} />}</button><p className="vote-note">Choose one option. One vote per browser.</p></>}</form></section>
      <aside className="results-column"><section className="panel results-card"><div className="results-heading"><h2>Results</h2><span className="vote-total"><Users size={15} />{poll.total.toLocaleString()} {poll.total === 1 ? 'vote' : 'votes'}</span></div><div className="result-list" aria-live="polite" aria-atomic="true">{poll.options.map((option, i) => { const percent = poll.total ? Math.round(option.votes / poll.total * 100) : 0; return <div className="result" key={option.id}><div className="result-label"><span>{option.label}</span><strong>{percent}%</strong></div><div className="result-track"><span style={{ width: `${percent}%`, background: colors[i] }} /></div><span className="result-count">{option.votes} {option.votes === 1 ? 'vote' : 'votes'}</span></div>; })}</div><p className="results-note">{poll.total === 0 ? 'The first vote gets things moving.' : poll.open ? 'Results update automatically.' : 'Final results · voting is closed'}</p></section><Share id={id} />{poll.isOwner && poll.open && <button className="close-button" onClick={close} disabled={closing}><LockKeyhole size={16} />{closing ? 'Closing…' : 'Close this poll'}</button>}</aside></div></>}
  </main><footer className="public-footer"><BarChart3 size={15} />Powered by PulsePoll</footer></div>;
}

function Share({ id }) {
  const [copied, setCopied] = useState(false), [failed, setFailed] = useState(false);
  const url = location.origin + '/poll/' + id;
  useEffect(() => { if (!copied) return; const timer = setTimeout(() => setCopied(false), 2500); return () => clearTimeout(timer); }, [copied]);
  async function copy() { try { await navigator.clipboard.writeText(url); setCopied(true); setFailed(false); } catch { setFailed(true); } }
  return <section className="share-card"><div className="share-icon"><Link2 size={19} /></div><h2>Bring everyone in</h2><p>Share this link with your audience.</p><div className="share-input"><input aria-label="Poll share link" value={url} readOnly onFocus={e => e.target.select()} /><button aria-label="Copy poll link" onClick={copy}>{copied ? <Check size={18} /> : <Copy size={18} />}</button></div><span className="copy-status" role="status">{copied ? 'Link copied!' : failed ? 'Select the link above and copy it manually.' : 'Anyone with the link can vote.'}</span></section>;
}

createRoot(document.getElementById('root')).render(<React.StrictMode><App /></React.StrictMode>);
