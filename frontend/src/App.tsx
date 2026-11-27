import { useState } from 'react';
import type { ReactElement } from 'react';
import { ChartWheel } from './components/ChartWheel';
import { PlanetGlyph } from './components/Glyph';
import { unavailableSignal } from './contracts';
import { isFreeTab } from './utils/feature-access';

const TABS = ['Overview', 'Chart interface', 'Social & access', 'Architecture'] as const;
type Tab = (typeof TABS)[number];

/** Whether the fictional friend has been asked to compare charts. */
type Consent = 'none' | 'requested';

/** Product tabs as the access policy names them, with the name a member sees. */
const FEATURES = [
  { tab: 'personal', label: 'Natal chart' },
  { tab: 'kabbalah', label: 'Personal Kabbalah' },
  { tab: 'market', label: 'General market' },
  { tab: 'backtest', label: 'Backtest research' },
  { tab: 'dashboard', label: 'Social dashboard' },
  { tab: 'crypto', label: 'Asset signals' },
  { tab: 'clanker', label: 'Trading automation' },
] as const;

function Sidebar({ tab, onSelect }: { tab: Tab; onSelect: (tab: Tab) => void }) {
  return (
    <aside>
      <a className="brand" href="https://astradia.xyz" target="_blank" rel="noreferrer">
        <span className="brand-mark">✧</span>
        <span>
          ASTRADIA<small>PUBLIC WORKSPACE</small>
        </span>
      </a>
      <div className="nav-heading">Engineering showcase</div>
      <nav aria-label="Public workspace">
        {TABS.map((name, i) => (
          <button key={name} aria-current={tab === name ? 'page' : undefined} onClick={() => onSelect(name)}>
            <span className="nav-number">0{i + 1}</span>
            {name}
          </button>
        ))}
      </nav>
      <div className="sidebar-note">
        <span className="status-dot" /> Source available for review
        <p>The trading engine remains private. This workspace runs without accounts, payments, or production services.</p>
      </div>
      <a className="repo-link" href="https://github.com/agacja/AstradiaPublic" target="_blank" rel="noreferrer">
        View repository ↗
      </a>
    </aside>
  );
}

function Overview({ onSelect }: { onSelect: (tab: Tab) => void }) {
  return (
    <>
      <section className="hero">
        <div className="eyebrow">An open view of the product</div>
        <h1>
          Explore the interface.
          <br />
          <em>Understand the architecture.</em>
        </h1>
        <p>
          Astradia connects personal astrology, social discovery, and market research. This repository shares the
          surrounding engineering while preserving the proprietary signal engine.
        </p>
        <div className="hero-actions">
          <button className="primary" onClick={() => onSelect('Chart interface')}>
            Explore the chart interface <span>↗</span>
          </button>
          <button className="secondary" onClick={() => onSelect('Architecture')}>
            See the system boundary
          </button>
        </div>
        <div className="orbit" aria-hidden="true">
          <div />
          <div />
          <div />
          <span>✧</span>
        </div>
      </section>
      <div className="section-title">
        <h2>Inside this workspace</h2>
        <span>Three technologies. One clear boundary.</span>
      </div>
      <div className="cards">
        <article>
          <span className="card-number">01 / FRONTEND</span>
          <h3>React & TypeScript</h3>
          <p>
            Product glyphs, chart presentation, free-access policy, account-scoped storage, and payment-state
            synchronization.
          </p>
          <span className="tag">Reusable components</span>
        </article>
        <article>
          <span className="card-number">02 / ASTRONOMY</span>
          <h3>Rust & Axum</h3>
          <p>
            The standalone astronomy service computes positions and houses. It carries no trading interpretations or
            strategy decisions.
          </p>
          <span className="tag">Independent service</span>
        </article>
        <article>
          <span className="card-number">03 / SOCIAL</span>
          <h3>Go & PostgreSQL</h3>
          <p>
            Extracted social persistence: profiles, friendships, consent, and query behavior. No customer database is
            included.
          </p>
          <span className="tag">Reviewable data layer</span>
        </article>
      </div>
      <section className="private-strip">
        <div className="private-icon">◇</div>
        <div>
          <h3>The proprietary engine stays private</h3>
          <p>
            Signal mathematics, weights, calibration, decision rules, and trading execution are intentionally excluded.
          </p>
        </div>
        <button className="text-button" onClick={() => onSelect('Architecture')}>
          How the pieces connect →
        </button>
      </section>
    </>
  );
}

function ChartInterface() {
  return (
    <>
      <div className="page-heading">
        <div className="eyebrow">Presentation / no scoring</div>
        <h1>A familiar view of the sky.</h1>
        <p>The product’s SVG glyph library in an interactive display study. Positions below are illustrative.</p>
      </div>
      <section className="panel">
        <ChartWheel />
      </section>
    </>
  );
}

interface SocialAndAccessProps {
  pro: boolean;
  onTogglePro: () => void;
  consent: Consent;
  onRequestConsent: () => void;
}

function SocialAndAccess({ pro, onTogglePro, consent, onRequestConsent }: SocialAndAccessProps) {
  return (
    <>
      <div className="page-heading">
        <div className="eyebrow">Product boundaries</div>
        <h1>Discovery, with clear consent.</h1>
        <p>A local interface preview. No real accounts, requests, memberships, or payments are changed.</p>
      </div>
      <div className="two-columns">
        <section className="panel">
          <div className="panel-title">
            <h3>Membership presentation</h3>
            <button className="secondary compact" onClick={onTogglePro}>
              {pro ? 'Pro preview' : 'Free preview'} ⇄
            </button>
          </div>
          <ul className="feature-list">
            {FEATURES.map(({ tab, label }) => {
              const included = pro || isFreeTab(tab);
              return (
                <li key={tab}>
                  <span>{label}</span>
                  <span className={included ? 'available' : 'locked'}>{included ? 'Included' : 'Pro'}</span>
                </li>
              );
            })}
          </ul>
          <p className="fine">Production access is enforced on the server. This switch only previews the interface.</p>
        </section>
        <section className="panel">
          <div className="eyebrow">Fictional social fixture</div>
          <div className="friend">
            <div className="avatar">
              <PlanetGlyph name="Moon" size={24} />
            </div>
            <div>
              <h3>Lyra</h3>
              <span>Example accepted friend</span>
            </div>
          </div>
          <p>Friendship and permission to compare charts are separate decisions.</p>
          <button className="primary" disabled={consent === 'requested'} onClick={onRequestConsent}>
            {consent === 'requested' ? 'Demo request pending' : 'Preview a comparison request'}
          </button>
          <p className="fine">
            No comparison runs until both people consent. The calculation service is outside this public workspace.
          </p>
          <div className="signal-placeholder">
            <span className="eyebrow">Private service boundary</span>
            <strong>Signal {unavailableSignal.state}</strong>
            <p>The public demo never invents a trading recommendation.</p>
          </div>
        </section>
      </div>
    </>
  );
}

function Architecture() {
  return (
    <>
      <div className="page-heading">
        <div className="eyebrow">Public components / private product</div>
        <h1>
          Visible by design.
          <br />
          Private at the core.
        </h1>
        <p>
          This is the production system’s conceptual separation. The demo does not connect to the private services.
        </p>
      </div>
      <section className="panel architecture">
        <div className="system-box">
          <span>PUBLIC SOURCE EXTRACTS</span>
          <h3>React interface</h3>
          <p>Presentation and public output contracts</p>
        </div>
        <div className="connector">Authenticated requests in production ↓</div>
        <div className="system-box private">
          <span>PRIVATE PRODUCTION PLATFORM</span>
          <h3>Rust API + shared signal engine</h3>
          <p>Identity · memberships · proprietary evaluation</p>
        </div>
        <div className="architecture-row">
          <div className="system-box">
            <span>PUBLIC SERVICE</span>
            <h3>Rust astronomy</h3>
            <p>Generic positions and houses</p>
          </div>
          <div className="system-box">
            <span>PUBLIC DATA-LAYER EXTRACT</span>
            <h3>Go social / PostgreSQL</h3>
            <p>Profiles, friendship and consent</p>
          </div>
          <div className="system-box private">
            <span>PRIVATE WORKER</span>
            <h3>Trading execution</h3>
            <p>Risk checks and exchange operations</p>
          </div>
        </div>
        <p className="fine">
          Both production Rust processes use the same proprietary engine library. There is no engine source, binary,
          strategy replay, or calibration table in this repository.
        </p>
      </section>
    </>
  );
}

export default function App() {
  // The preview's state lives here, so it survives a change of tab.
  const [tab, setTab] = useState<Tab>('Overview');
  const [pro, setPro] = useState(false);
  const [consent, setConsent] = useState<Consent>('none');
  // Every tab has a page: a tab added to TABS without a case here does not compile.
  function page(): ReactElement {
    switch (tab) {
      case 'Overview':
        return <Overview onSelect={setTab} />;
      case 'Chart interface':
        return <ChartInterface />;
      case 'Social & access':
        return (
          <SocialAndAccess
            pro={pro}
            onTogglePro={() => setPro(current => !current)}
            consent={consent}
            onRequestConsent={() => setConsent('requested')}
          />
        );
      case 'Architecture':
        return <Architecture />;
    }
  }
  return (
    <div className="workspace">
      <Sidebar tab={tab} onSelect={setTab} />
      <main>
        <header>
          <span>
            Astradia / <strong>{tab}</strong>
          </span>
          <span className="badge">PUBLIC EDITION</span>
        </header>
        {page()}
        <footer>
          <span>ASTRADIA · Public engineering workspace</span>
          <span>Illustrative UI · No live trading</span>
        </footer>
      </main>
    </div>
  );
}
