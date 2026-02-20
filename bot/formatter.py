import html

from scrapers.base import Post


def format_single_post(post: Post) -> str:
    """개별 게시글 메시지 생성
    예: [인벤] 게시물제목
    https://www.inven.co.kr/...
    """
    safe_title = html.escape(post.title)
    return f"[{post.community_name}] {safe_title}\n{post.url}"
