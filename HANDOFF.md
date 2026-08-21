# Project Handoff: ToS Legality Research

## August 21, 2026

I have completed the legal and regulatory research regarding the Terms of Service (ToS) legality of automated opt-out submissions. The findings are documented in full within `docs/TOS-LEGALITY-FINDINGS.md`. 

Below is a summary of the findings, unresolved open questions, and the complete citation of sources used.

---

### 1. Summary of Findings

*   **Negligible Legal Risk for Individuals:** Running a local browser automation tool (like `unbrokerrdd`) to submit opt-out and deletion requests for yourself carries negligible to zero realistic legal liability. While it technically violates boilerplate website ToS clauses prohibiting "automated access" or "bots," there is no viable claim under federal hacking laws (CFAA) or state contract laws. Because the consumer is exercising a statutory, non-waivable privacy right and data brokers suffer no operational damages, civil lawsuits against individual users are non-existent.
*   **Authorized Agent Regulations (CCPA):** Under California regulation 11 CCR § 7063, businesses cannot require consumers to verify their identity for opt-out/limit-use requests submitted by an agent. However, they can require identity verification for full deletion/know requests. 
*   **The Critical Boundary — Local vs. Hosted:** Running the tool locally is a direct exercise of consumer rights using an assistive script. Hosting this as a service for others makes the operator an "Authorized Agent." This requires formal business registration with the California Secretary of State, written signed permissions from customers, and invites heavy technical and legal opposition from brokers.
*   **The California Delete Act (SB 362) / DROP Platform:** The official platform at [privacy.ca.gov/drop](https://privacy.ca.gov/drop) is fully live as of 2026, with mandatory broker enforcement having kicked in on **August 1, 2026**. It allows California residents to delete their data across ~600 brokers with one click, but has a 45-to-90-day delay and is entirely unavailable to residents of the other 49 states. `unbrokerrdd` serves as an essential immediate alternative for Californians and a primary solution for non-Californians.
*   **Precedent from Commercial Competitors:** Large-scale commercial opt-out bots (Optery, Incogni, DeleteMe) have never been sued by brokers over ToS/automation. Brokers instead rely on technical blocks (CAPTCHAs/IP gates). Regulators (the CPPA) are actively fighting these "dark patterns," recently fining broker **LocateSmarter LLC $116,490** in August 2026 for obstructing agent/automated requests.

---

### 2. What We Couldn't Determine / Open Questions

*   **Actual Broker Compliancy Rates:** While the DROP platform is live, it is too early to determine the exact long-term compliance rates of all ~600 registered brokers. Early CPPA reports indicate around 30% processed requests in the first week of mandatory enforcement, but the exact lag/compliance curve for smaller brokers remains undetermined.
*   **Future State Platform Rollouts:** Whether other states with active comprehensive privacy laws (such as Virginia, Colorado, Texas, or Oregon) will follow California's model by establishing unified deletion platforms of their own, and the specific timelines for such platforms.
*   **Private Whitelist Agreements:** Whether commercial removal services (like Optery or DeleteMe) have entered into confidential private-contract whitelisting agreements with major data broker consortia (like PeopleConnect) to allow their bots to bypass CAPTCHAs, or if they rely solely on advanced anti-fingerprinting proxies and manual backup agents.

---

### 3. Sources Actually Used

1.  **Statutes & Regulations:**
    *   **Computer Fraud and Abuse Act (CFAA):** 18 U.S.C. § 1030.
    *   **California Consumer Privacy Act (CCPA):** California Civil Code § 1798.135 (Opt-out/Limit link requirements) and § 1798.192 (Non-waivability of rights).
    *   **CCPA Regulations on Authorized Agents:** California Code of Regulations (CCR) Title 11, Section 7063.
    *   **California Delete Act (SB 362):** Enacted in 2023, modifying data broker registration and deletion platform provisions (Cal. Civ. Code § 1798.99.80 et seq.).

2.  **Case Law Precedents:**
    *   ***hiQ Labs, Inc. v. LinkedIn Corp.*, 31 F.4th 1180 (9th Cir. 2022):** Landmark 9th Circuit ruling establishing that scraping publicly available data does not violate the CFAA.
    *   ***hiQ Labs, Inc. v. LinkedIn Corp.*, U.S. District Court, N.D. Cal. (Nov 2022) / Consent Judgment (Dec 2022):** District Court ruling on breach of contract finding that ToS are enforceable contracts if signed or agreed to via account creation, leading to a $500,000 settlement and permanent injunction.

3.  **Regulatory Announcements & Portals:**
    *   **California Privacy Protection Agency (CPPA) DROP Portal:** [privacy.ca.gov/drop](https://privacy.ca.gov/drop) (Live platform status and residency verification gates, August 2026).
    *   **CPPA Enforcement Action (August 2026):** Administrative fine of $116,490 against *LocateSmarter LLC* for non-registration and obstruction of automated/agent opt-out requests.

4.  **Data Broker policies & Technical references:**
    *   **PeopleConnect (TruthFinder Parent) Terms of Use:** Section 1.3.6 (Explicit bot and automated access prohibition) at [peopleconnect.us](https://peopleconnect.us) / [suppression.peopleconnect.us](https://suppression.peopleconnect.us).
    *   **Optery's "Dishonorable Data Broker List":** Optery reports on data brokers using technical obstruction to evade automated consumer opt-out submissions.
    *   **Incogni GDPR Article 17 Filings:** Surfshark/Incogni public legal complaints filed with European data protection authorities regarding broker non-compliance with Right to Be Forgotten requests.
