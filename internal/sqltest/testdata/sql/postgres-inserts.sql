--
-- PostgreSQL database dump (pg_dump --inserts --column-inserts)
--

\restrict aB3cD5eF7gH9jK2lM4nP6qR8sT0uV1wX3yZ5aB7cD9eF2gH4jK6lM8nP0qR2sT4u

-- Dumped from database version 17.6
-- Dumped by pg_dump version 17.6

SET statement_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET client_min_messages = warning;

--
-- Name: touch_updated_at(); Type: FUNCTION; Schema: public; Owner: shop
--

CREATE FUNCTION public.touch_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  -- a 'quoted' comment; and a "double" one
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;


ALTER FUNCTION public.touch_updated_at() OWNER TO shop;

SET default_tablespace = '';

SET default_table_access_method = heap;

CREATE TABLE public.users (
    id bigint NOT NULL,
    name character varying(255) NOT NULL,
    email character varying(255) NOT NULL,
    password character varying(255) NOT NULL,
    remember_token character varying(100),
    avatar bytea,
    created_at timestamp(0) without time zone
);


ALTER TABLE public.users OWNER TO shop;

CREATE TABLE public.orders (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    billing_email character varying(255) NOT NULL,
    notes text,
    meta jsonb
);


ALTER TABLE public.orders OWNER TO shop;

--
-- Data for Name: users; Type: TABLE DATA; Schema: public; Owner: shop
--

INSERT INTO public.users (id, name, email, password, remember_token, avatar, created_at) VALUES (1, 'Alice O''Brien', 'alice@shop.test', '$2y$12$Lk4mN7pQ1rS3tU5vW8xY0zA2bC4dE6fG8hI0jK2lM4nO6pQ8rS0tU', '{{fake 60 8}}', '\x89504e470d0a1a0a', '2026-10-01 09:00:00');
INSERT INTO public.users (id, name, email, password, remember_token, avatar, created_at) VALUES (2, E'Bob "Bobby"\nJones', 'bob@shop.test', E'$2y$12$Pz9yX8wV7uT6sR5qP4oN3mL2kJ1iH0gF9eD8cB7aZ6yX5wV4uT3s', NULL, NULL, '2026-10-02 10:00:00');
INSERT INTO public.users (id, name, email, password, remember_token, avatar, created_at) VALUES (3, 'Carys Llŷr', 'carys@shop.test', $q$$2y$12$Qa1sW2dE3rF4tG5yH6uJ7iK8oL9pZ0xC1vB2nM3qW4eR5tY6uI7o$q$, NULL, NULL, '2026-10-03 12:00:00');


--
-- Data for Name: orders; Type: TABLE DATA; Schema: public; Owner: shop
--

INSERT INTO public.orders (id, user_id, billing_email, notes, meta) VALUES (1, 1, 'alice@shop.test', E'Back door\\side gate, call 07700 900123', '{"gateway": "stripe", "secret": "sk_live_{{fake 24 2}}"}');
INSERT INTO public.orders (id, user_id, billing_email, notes, meta) VALUES (2, 2, 'bob@shop.test', 'Refund to card 4242 4242 4242 4242
from 198.51.100.23', NULL);


--
-- PostgreSQL database dump complete
--

\unrestrict aB3cD5eF7gH9jK2lM4nP6qR8sT0uV1wX3yZ5aB7cD9eF2gH4jK6lM8nP0qR2sT4u

