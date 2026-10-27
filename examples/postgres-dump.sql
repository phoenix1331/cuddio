--
-- PostgreSQL database dump
--

\restrict pXfK2mQ8vT4wZ7yB1nC5dG9hJ3kL6sR0aE2uI8oP4qW7eY1tV5xN9mB3cF6gH0j

-- Dumped from database version 17.6
-- Dumped by pg_dump version 17.6

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: pgcrypto; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;


--
-- Name: EXTENSION pgcrypto; Type: COMMENT; Schema: -; Owner:
--

COMMENT ON EXTENSION pgcrypto IS 'cryptographic functions';


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: orders; Type: TABLE; Schema: public; Owner: shop
--

CREATE TABLE public.orders (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    number character varying(20) NOT NULL,
    status character varying(255) DEFAULT 'pending'::character varying NOT NULL,
    total numeric(10,2) NOT NULL,
    billing_name character varying(255) NOT NULL,
    billing_email character varying(255) NOT NULL,
    billing_phone character varying(32),
    shipping_address text,
    shipping_postcode character varying(10),
    card_last4 character(4),
    payment_intent character varying(64),
    notes text,
    meta jsonb,
    created_at timestamp(0) without time zone,
    updated_at timestamp(0) without time zone
);


ALTER TABLE public.orders OWNER TO shop;

--
-- Name: orders_id_seq; Type: SEQUENCE; Schema: public; Owner: shop
--

CREATE SEQUENCE public.orders_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.orders_id_seq OWNER TO shop;

ALTER SEQUENCE public.orders_id_seq OWNED BY public.orders.id;


--
-- Name: password_reset_tokens; Type: TABLE; Schema: public; Owner: shop
--

CREATE TABLE public.password_reset_tokens (
    email character varying(255) NOT NULL,
    token character varying(255) NOT NULL,
    created_at timestamp(0) without time zone
);


ALTER TABLE public.password_reset_tokens OWNER TO shop;

--
-- Name: personal_access_tokens; Type: TABLE; Schema: public; Owner: shop
--

CREATE TABLE public.personal_access_tokens (
    id bigint NOT NULL,
    tokenable_type character varying(255) NOT NULL,
    tokenable_id bigint NOT NULL,
    name text NOT NULL,
    token character varying(64) NOT NULL,
    abilities text,
    last_used_at timestamp(0) without time zone,
    expires_at timestamp(0) without time zone,
    created_at timestamp(0) without time zone,
    updated_at timestamp(0) without time zone
);


ALTER TABLE public.personal_access_tokens OWNER TO shop;

--
-- Name: sessions; Type: TABLE; Schema: public; Owner: shop
--

CREATE TABLE public.sessions (
    id character varying(255) NOT NULL,
    user_id bigint,
    ip_address character varying(45),
    user_agent text,
    payload text NOT NULL,
    last_activity integer NOT NULL
);


ALTER TABLE public.sessions OWNER TO shop;

--
-- Name: users; Type: TABLE; Schema: public; Owner: shop
--

CREATE TABLE public.users (
    id bigint NOT NULL,
    name character varying(255) NOT NULL,
    email character varying(255) NOT NULL,
    email_verified_at timestamp(0) without time zone,
    password character varying(255) NOT NULL,
    remember_token character varying(100),
    avatar bytea,
    created_at timestamp(0) without time zone,
    updated_at timestamp(0) without time zone
);


ALTER TABLE public.users OWNER TO shop;

--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: shop
--

CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.users_id_seq OWNER TO shop;

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: orders id; Type: DEFAULT; Schema: public; Owner: shop
--

ALTER TABLE ONLY public.orders ALTER COLUMN id SET DEFAULT nextval('public.orders_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: shop
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Data for Name: orders; Type: TABLE DATA; Schema: public; Owner: shop
--

COPY public.orders (id, user_id, number, status, total, billing_name, billing_email, billing_phone, shipping_address, shipping_postcode, card_last4, payment_intent, notes, meta, created_at, updated_at) FROM stdin;
1	1	SO-1001	paid	42.50	Alice O'Brien	alice@example.com	07700 900123	12 Heol y Frenhines\nCaerdydd	CF10 1AA	4242	pi_f7Gh9Jk2Lm4Np6Qr8St0Uv1W	Customer said: "leave with neighbour"	{"gateway": "stripe", "secret": "2Lm4Np6Qr8St0Uv1WxAb3Cd5Ef7Gh9Jk"}	2026-10-01 09:15:00	2026-10-01 09:16:02
2	2	SO-1002	refunded	19.99	Bob "Bobby" Jones	bob@example.com	+44 7700 900456	Flat 3, 8 Mill Lane\\Rear entrance	SW1A 1AA	\N	pi_Qr8St0Uv1WxAb3Cd5Ef7Gh9J	Refund to card 4242 4242 4242 4242,\tcalled from 198.51.100.23	{"gateway": "stripe"}	2026-10-02 14:00:00	2026-10-03 08:30:00
3	1	SO-1003	pending	7.00	Alice O'Brien	alice@example.com	\N	\N	\N	\N	\N		\N	2026-10-04 18:45:10	\N
\.


--
-- Data for Name: password_reset_tokens; Type: TABLE DATA; Schema: public; Owner: shop
--

COPY public.password_reset_tokens (email, token, created_at) FROM stdin;
bob@example.com	$2y$12$Xq3vT8kLm2Rp9sWd4Fh6JuN1bC7eA5gH0iK3lM8nO2pQ4rS6tU9vW	2026-10-05 07:00:00
\.


--
-- Data for Name: personal_access_tokens; Type: TABLE DATA; Schema: public; Owner: shop
--

COPY public.personal_access_tokens (id, tokenable_type, tokenable_id, name, token, abilities, last_used_at, expires_at, created_at, updated_at) FROM stdin;
1	App\\Models\\User	1	cli	56789abcdef0123456789abcdef0123456789abcdef0123456789abcdef01234	["*"]	2026-10-20 11:02:33	\N	2026-10-01 09:00:00	2026-10-20 11:02:33
2	App\\Models\\User	2	mobile app	abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789	["orders:read"]	\N	2027-01-01 00:00:00	2026-10-02 10:00:00	2026-10-02 10:00:00
\.


--
-- Data for Name: sessions; Type: TABLE DATA; Schema: public; Owner: shop
--

COPY public.sessions (id, user_id, ip_address, user_agent, payload, last_activity) FROM stdin;
v1WxAb3Cd5Ef7Gh9Jk2Lm4Np6Qr8St0Uv1WxAb3C	1	203.0.113.9	Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36	YTozOntzOjY6Il90b2tlbiI7czo0MDoiCd5Ef7Gh9Jk2Lm4Np6Qr8St0Uv1WxAb3Cd5Ef7Gh9Jk2Lm4Np6Qr8St0Uv1WxAb3Cd5Ef7Gh9Jk2Lm4N	1793000000
h9Jk2Lm4Np6Qr8St0Uv1WxAb3Cd5Ef7Gh9Jk2Lm4	\N	2001:db8::7	curl/8.5.0	YToxOntzOjY6Il90b2tlbiI7czo0MDoi4Np6Qr8St0Uv1WxAb3Cd5Ef7Gh9Jk2Lm4Np6Qr8St0Uv1WxAb3Cd5Ef7	1793000100
\.


--
-- Data for Name: users; Type: TABLE DATA; Schema: public; Owner: shop
--

COPY public.users (id, name, email, email_verified_at, password, remember_token, avatar, created_at, updated_at) FROM stdin;
1	Alice O'Brien	alice@example.com	2026-10-01 09:00:00	$2y$12$Lk4mN7pQ1rS3tU5vW8xY0zA2bC4dE6fG8hI0jK2lM4nO6pQ8rS0tU	St0Uv1WxAb3Cd5Ef7Gh9Jk2Lm4Np6Qr8St0Uv1WxAb3Cd5Ef7Gh9Jk2Lm4Np	\\x89504e470d0a1a0a	2026-10-01 09:00:00	2026-10-01 09:00:00
2	Bob "Bobby" Jones	bob@example.com	\N	$2y$12$Pz9yX8wV7uT6sR5qP4oN3mL2kJ1iH0gF9eD8cB7aZ6yX5wV4uT3s	xAb3Cd5Ef7Gh9Jk2Lm4Np6Qr8St0Uv1WxAb3Cd5Ef7Gh9Jk2Lm4Np6Qr8St0	\N	2026-10-02 10:00:00	2026-10-05 07:00:00
3	Carys Llŷr	carys@example.com	\N	$2y$12$Qa1sW2dE3rF4tG5yH6uJ7iK8oL9pZ0xC1vB2nM3qW4eR5tY6uI7o	\N	\N	2026-10-03 12:00:00	2026-10-03 12:00:00
\.


--
-- Name: orders_id_seq; Type: SEQUENCE SET; Schema: public; Owner: shop
--

SELECT pg_catalog.setval('public.orders_id_seq', 3, true);


--
-- Name: users_id_seq; Type: SEQUENCE SET; Schema: public; Owner: shop
--

SELECT pg_catalog.setval('public.users_id_seq', 3, true);


--
-- Name: orders orders_pkey; Type: CONSTRAINT; Schema: public; Owner: shop
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT orders_pkey PRIMARY KEY (id);


--
-- Name: password_reset_tokens password_reset_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: shop
--

ALTER TABLE ONLY public.password_reset_tokens
    ADD CONSTRAINT password_reset_tokens_pkey PRIMARY KEY (email);


--
-- Name: sessions sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: shop
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: shop
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users_email_unique; Type: INDEX; Schema: public; Owner: shop
--

CREATE UNIQUE INDEX users_email_unique ON public.users USING btree (email);


--
-- Name: orders orders_user_id_foreign; Type: FK CONSTRAINT; Schema: public; Owner: shop
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT orders_user_id_foreign FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

\unrestrict pXfK2mQ8vT4wZ7yB1nC5dG9hJ3kL6sR0aE2uI8oP4qW7eY1tV5xN9mB3cF6gH0j

