import DOMPurify from 'dompurify';

const allowedTags = [
  'a',
  'blockquote',
  'br',
  'code',
  'del',
  'em',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'hr',
  'img',
  'li',
  'ol',
  'p',
  'pre',
  'span',
  'strong',
  'table',
  'tbody',
  'td',
  'th',
  'thead',
  'tr',
  'ul',
];

const allowedAttributes = ['alt', 'class', 'href', 'rel', 'src', 'target', 'title'];
const allowedUriRegexp =
  /^(?:(?:(?:f|ht)tps?|mailto|tel):|[^a-z]|[a-z+.-]+(?:[^a-z+.\-:]|$))/i;

let hooksConfigured = false;

function configureDOMPurify() {
  if (hooksConfigured || typeof window === 'undefined') {
    return;
  }

  DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    if (!(node instanceof window.Element)) {
      return;
    }

    for (const attributeName of ['href', 'src']) {
      const value = node.getAttribute(attributeName);
      if (value?.trim().startsWith('//')) {
        node.removeAttribute(attributeName);
      }
    }

    if (
      node.tagName.toLowerCase() === 'a' &&
      node.getAttribute('target') === '_blank'
    ) {
      node.setAttribute('rel', 'noreferrer noopener');
    }
  });

  hooksConfigured = true;
}

export function sanitizeHtml(html: string) {
  if (typeof window === 'undefined') {
    return '';
  }

  configureDOMPurify();

  return DOMPurify.sanitize(html, {
    ALLOWED_ATTR: allowedAttributes,
    ALLOWED_TAGS: allowedTags,
    ALLOW_DATA_ATTR: false,
    ALLOW_UNKNOWN_PROTOCOLS: false,
    FORBID_ATTR: ['style'],
    FORBID_TAGS: ['form', 'iframe', 'math', 'script', 'svg'],
    RETURN_TRUSTED_TYPE: false,
    SAFE_FOR_TEMPLATES: true,
    ALLOWED_URI_REGEXP: allowedUriRegexp,
  });
}
