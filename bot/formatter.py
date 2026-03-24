from scrapers.base import Post


def format_single_post(post: Post) -> str:
    """개별 게시글 메시지 생성
    예: [인벤] 게시물제목 ❤84 👁12,000
    https://www.inven.co.kr/...
    """
    title = post.title
    stats = []
    if post.votes > 0:
        stats.append(f"❤{post.votes}")
    if post.views > 0:
        stats.append(f"👁{post.views:,}")
    if post.comments > 0:
        stats.append(f"💬{post.comments}")

    suffix = f" {' '.join(stats)}" if stats else ""
    return f"[{post.community_name}] {title}{suffix}\n{post.url}"
