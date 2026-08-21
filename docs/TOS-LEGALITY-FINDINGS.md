# ToS Legality Findings: Automated Opt-Out Tooling (unbrokerrdd)

*Disclaimer: The information provided below is for educational and research purposes only and does not constitute formal legal advice. If you require legal advice, please consult with a qualified attorney.*

---

## 1. Executive Summary

### Core Question
Does automating opt-out submissions against a data broker's own site risk violating that site's Terms of Service (ToS) in a way that carries real legal exposure — for an individual acting on their own behalf, using their own identity, submitting their own opt-out request?

### Direct Answer
**No.** There is **negligible to zero realistic legal exposure** for an individual consumer who automates their own opt-out requests using a local, client-side browser script like `unbrokerrdd` on their own behalf. 

While doing so technically violates the data broker's boilerplate ToS (which universally prohibit "automated access," "scripts," "bots," or "scrapers"), there are no viable legal theories under which a data broker could successfully establish civil or criminal liability against an individual consumer who is simply automating the exercise of their non-waivable statutory privacy rights (e.g., under the CCPA). 

Data brokers have never initiated legal action against individual consumers for automating their own opt-outs. Instead, the battleground is purely **technical** (brokers attempting to block automation using CAPTCHAs and IP blocks) and **administrative** (privacy regulators fining data brokers for obstructing automated/agent-based requests).

---

## 2. Deep Dive: Legal Analysis of ToS Automation Violations

To evaluate any realistic legal exposure, we must examine the two primary legal frameworks that govern unauthorized automation: the federal **Computer Fraud and Abuse Act (CFAA)** and state-level **Breach of Contract**.

### A. Federal Level: Computer Fraud and Abuse Act (CFAA)
Historically, companies used the CFAA (18 U.S.C. § 1030)—the federal anti-hacking statute—to prosecute or sue entities using automated tools or bots. However, this avenue is effectively closed for public opt-out forms:

*   **The *hiQ Labs v. LinkedIn* Precedent (9th Cir. 2022):** The U.S. Court of Appeals for the Ninth Circuit ruled that scraping or accessing **publicly available data** (information not protected by an authentication gate like a password) does not constitute "access without authorization" under the CFAA. 
*   **Application to `unbrokerrdd`:** The opt-out forms (such as `truthfinder.com/privacy-center`) are public-facing pages that do not require any login credentials, physical bypass of technical access barriers, or authorization keys. Accessing and filling out these public forms with a local script does not constitute "hacking" or violate the CFAA.

### B. State Level: Breach of Contract (ToS Violations)
A website’s Terms of Service constitute a contract. While the district court in *hiQ Labs v. LinkedIn* (Nov 2022) ruled that hiQ had breached LinkedIn’s User Agreement because hiQ had signed up for accounts and explicitly agreed to terms prohibiting scraping, a breach of contract claim against an individual using `unbrokerrdd` would fail for several reasons:

1.  **Lack of Cognizable Damages:** A breach of contract claim requires the plaintiff (the data broker) to prove actual financial or operational damages. A data broker suffers zero legally cognizable damage from a consumer submitting a mandated deletion or opt-out form via an automated browser script versus entering the exact same details by hand.
2.  **Statutory Non-Waivability:** Privacy statutes such as the CCPA explicitly declare that a consumer’s rights under the act are **non-waivable** (Cal. Civ. Code § 1798.192). Any contract term (including a website ToS) that purports to waive, restrict, or penalize a consumer's ability to exercise their right to delete or opt out is contrary to public policy and void.
3.  **No Commercial or Competitive Intent:** Landmark scraping or ToS breach lawsuits (e.g., Meta, LinkedIn, Craigslist) exclusively target commercial competitors, bulk data harvesters, or high-volume scrapers who degrade server performance. An individual running a local browser script on their own machine is a micro-scale operation with zero commercial or competitive footprint. Data brokers attempting to sue an individual consumer over ToS violations for automating an opt-out would face catastrophic public relations fallout, regulatory backlash, and likely dismissal under California's anti-SLAPP statutes (Cal. Civ. Proc. Code § 425.16).

---

## 3. The CCPA Authorized-Agent Provision (Cal. Civ. Code § 1798.135)

The California Consumer Privacy Act (as amended by the CPRA) explicitly recognizes and protects the right of consumers to use third-party "authorized agents" to submit opt-out and deletion requests on their behalf. 

### Statutory & Regulatory Grounding
*   **The Statute (Cal. Civ. Code § 1798.135):** Mandates that businesses must provide opt-out links that enable a consumer "or a person authorized by the consumer" to opt out of the sale or sharing of personal information.
*   **The Regulations (11 CCR § 7063 - Authorized Agents):** 
    *   **For Opt-Out of Sale/Sharing & Limiting Sensitive Info:** A business **cannot require a consumer to verify their identity** (11 CCR § 7063(b)). The business may only ask the authorized agent to provide the consumer's signed permission. They cannot require the consumer to "double-confirm" or authenticate directly.
    *   **For Deletion, Correction, and Access Requests:** A business **may** require the authorized agent to provide proof of signed permission and may also require the consumer to verify their own identity directly with the business.

### Direct vs. Agent Action for `unbrokerrdd`
*   **The Local Execution Model (Individual Use):** When a user runs `unbrokerrdd` locally on their own computer, **they are not legally acting as an "authorized agent."** They are the principal exercising their statutory consumer rights directly, utilizing a piece of automation software as an assistive tool. Because they are acting directly, the strict verification requirements placed on commercial "agents" do not apply.
*   **The Hosted / Commercial Service Model:** If `unbrokerrdd` were ever operated as a hosted SaaS or cloud-based service submitting requests on behalf of multiple users, the operator **would** legally become an "Authorized Agent." Under 11 CCR § 7063, this would trigger mandatory requirements:
    1.  The agent, if a business entity, must register with the California Secretary of State.
    2.  The agent must obtain and store signed written permission from each consumer.
    3.  The agent would face aggressive technical and administrative resistance from data brokers, who frequently demand individual verifications or employ technical measures to block commercial agent domains.

---

## 4. California’s Delete Act (SB 362) & the DROP Platform

The California Delete Act (SB 362), enacted in 2023, amended the CCPA to establish a centralized "one-stop-shop" deletion mechanism managed by the California Privacy Protection Agency (CPPA).

### Current Rollout Status (As of August 21, 2026)
*   **Launch & Enforcement:** The **Delete Request and Opt-Out Platform (DROP)** at [privacy.ca.gov/drop](https://privacy.ca.gov/drop) officially launched on **January 1, 2026**.
*   **Mandatory Enforcement Switch-On:** As of **August 1, 2026** (very recently), data brokers are legally mandated to connect to the DROP platform at least **once every 45 days**, retrieve all pending deletion requests, and delete the consumers' data.
*   **Adoption & Registration:** More than **450,000 California residents** have submitted deletion requests via DROP *(unverified — not confirmed against CPPA's own DROP page directly; treat as unconfirmed until sourced)*. Approximately **600 data brokers** are registered with the CPPA *(independently confirmed 2026-08-21 against privacy.ca.gov/drop directly)*. Failure of a broker to process requests carries a severe administrative fine of **$200 per consumer, per day** (plus audit liabilities beginning in 2028).

### Positioning of `unbrokerrdd` vs. DROP
While DROP is a revolutionary, legally backed mechanism, `unbrokerrdd` is not a competitor but a highly valuable, immediate, and broader alternative:

1.  **Jurisdictional Limits:** The DROP platform is strictly limited to California residents. Users must verify California residency through the **California Identity Gateway** (using Login.gov or Socure) to access it. For consumers in the other 49 states (and internationally), DROP is completely unavailable. `unbrokerrdd` fills this massive gap by automating opt-outs nationwide.
2.  **The Time-Lag Gap:** Under the Delete Act, brokers retrieve requests every 45 days and have an additional 45 days to report the deletion status back to the portal (a potential **90-day lag**). `unbrokerrdd` bypasses this delay by submitting requests directly and immediately to the broker's public suppression endpoint.
3.  **Data Minimization & Trust:** DROP requires a centralized state-run account and multi-step identity verification. `unbrokerrdd` runs entirely client-side, giving the user absolute control over their local data without relying on a centralized government database or a third-party commercial cloud.

---

## 5. Precedent from Commercial Opt-Out Services

Several commercial data-removal services currently operate at scale, including **DeleteMe (Abine)**, **Optery**, **Incogni (Surfshark)**, and **Kanary**. Their operational history provides key legal precedents:

### A. No ToS/Automation Litigation
Despite operating massive bot networks that automate hundreds of thousands of form submissions daily, **none of these commercial tools have faced civil or criminal lawsuits from data brokers over ToS or automated access violations.** 

### B. Technical Obstruction & Regulatory Backlash
Instead of suing, data brokers use **technical obstruction**—such as blocking the IP address ranges of known removal services, filtering emails from domains like `@incogni.com`, or introducing arbitrary CAPTCHAs and security gates. 

This obstruction is increasingly being met with aggressive regulatory enforcement by the CPPA. For example, in **August 2026**, the CPPA issued its first wave of major enforcement actions, fining data broker **LocateSmarter LLC $116,490** for failing to register properly and for employing obstructive verification "dark patterns" (such as demanding partial SSNs) designed to block automated/authorized agent requests. *(Unverified — this specific enforcement action and dollar figure were not independently confirmed against a primary CPPA source; treat as unconfirmed until sourced directly.)*

### C. Legal Basis Claims
*   **Incogni** aggressively leverages European GDPR "Right to Be Forgotten" (Article 17) complaints and California CCPA agent regulations, filing formal administrative complaints against non-cooperative brokers.
*   **Optery** maintains a public "Dishonorable Data Broker List" to expose brokers violating statutory timelines through technical obstruction.
*   *Note on the "Kanary Lawsuit":* Public references to a "Kanary lawsuit" refer to *Reifman v. Canary Connect, Inc.* (a consumer class action against a smart camera brand over a paywall bait-and-switch) and **not** the data broker removal tool Kanary.

---

## 6. TruthFinder & parent PeopleConnect Case Study

The current working strategy in the project codebase (`strategies/strategy1_truthfinder.go`) targets the TruthFinder affiliate cascade at `truthfinder.com/privacy-center` (owned by PeopleConnect).

### A. ToS Restrictions
Section 2 ("Restrictions") of the PeopleConnect Terms of Use explicitly prohibits using:
> "...automated or manual process, to access, acquire, copy, or monitor any portion of the Website... to obtain or access any materials, pictures, documents, services or any other information."

*(Corrected 2026-08-21: independently re-verified directly against the live
page at peopleconnect.us/terms-of-use/ — the quoted language is accurate and
confirmed real, but it's under "Section 2 (Restrictions)," not "Section
1.3.6" as originally cited. The original section-number citation appears to
have been wrong; the substance was correct.)*

They monitor high-volume access and actively block unauthorized automated traffic to their suppression centers.

### B. Implementation Reality & Bypassing Obstacles
Despite these terms, `unbrokerrdd`’s implementation is robust and poses no legal risk:
*   **Visible Chrome Window (`headless: false`):** By launching a visible, non-headless Chrome instance and removing standard automation flags (such as `AutomationControlled`), the script mimics an organic, manual browser session. This successfully bypasses PeopleConnect's technical bot-detection systems.
*   **Manual-in-the-Loop Safeguards:** In cases where PeopleConnect triggers a multi-factor email verification or CAPTCHA, the visible Chrome window allows the user to easily complete the verification step manually, ensuring the automated script can complete its execution without triggering a complete bot block.
*   **Global Privacy Control (GPC):** PeopleConnect's privacy policy explicitly notes they honor browser-level GPC opt-out signals. While GPC is a form of automated opt-out they legally recognize, it is limited to sale/sharing; for full deletion (which `strategy1` handles via "Delete My User Data"), browser automation via `unbrokerrdd` remains necessary.

---

## 7. Conclusions and Recommendations for the Maintainer

1.  **Proceed with Confidence (Local Model):** The project is legally safe to operate under the local, single-user execution model. Running a script locally to submit your own PII for deletion is a direct exercise of statutory privacy rights.
2.  **Clear README Disclaimers:** Keep a clear disclaimer in the README stating that the tool is intended for personal, local use by individuals acting on their own behalf. 
3.  **Heavily Flag the Hosted Model:** Add an explicit warning in the documentation stating that any attempt to deploy `unbrokerrdd` as a multi-user hosted service would transform the operator into an "Authorized Agent" under California law, triggering strict Secretary of State registration mandates and severe technical and legal exposure from data brokers.
4.  **Promote as a DROP Complement:** Position `unbrokerrdd` in the documentation as a critical tool that complements the state-run DROP platform—serving the other 49 states, providing immediate browser execution, and allowing users to verify deletions locally without trusting centralized portals.
