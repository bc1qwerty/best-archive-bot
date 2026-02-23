"""스크래퍼 스케줄 정의 — (스크래퍼, 주기(분), 첫 실행 지연(초))"""

from scrapers.base import BaseScraper
from scrapers.dcinside import DcinsideScraper
from scrapers.clien import ClienScraper
from scrapers.ppomppu import PpomppuScraper
from scrapers.ruliweb import RuliwebScraper
from scrapers.inven import InvenScraper
from scrapers.humoruniv import HumorunivScraper
from scrapers.theqoo import TheqooScraper
from scrapers.natepann import NatepannScraper
from scrapers.bobaedream import BobaedreamScraper
from scrapers.cook82 import Cook82Scraper
from scrapers.mlbpark import MlbparkScraper
from scrapers.etoland import EtolandScraper
from scrapers.dvdprime import DvdprimeScraper

SCRAPER_SCHEDULE: list[tuple[BaseScraper, int, int]] = [
    # --- 실시간 베스트 (10분) ---
    (DcinsideScraper(),  10,  10),
    (TheqooScraper(),    10,  50),
    (NatepannScraper(),  10,  90),

    # --- 일반 인기글 (15분) ---
    (ClienScraper(),     15, 130),
    (BobaedreamScraper(),15, 170),
    (MlbparkScraper(),   15, 210),
    (PpomppuScraper(),   15, 250),

    # --- 교체 느린 사이트 (20분) ---
    (RuliwebScraper(),   20, 290),
    (InvenScraper(),     20, 330),
    (Cook82Scraper(),    20, 370),
    (HumorunivScraper(), 20, 410),

    # --- 봇 차단 주의 ---
    # FmkoreaScraper: 비활성화 (클라우드 IP WAF 차단)
    (EtolandScraper(),   20, 450),
    (DvdprimeScraper(),  20, 490),
]
